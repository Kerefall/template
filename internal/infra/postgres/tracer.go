package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const meterName = "github.com/Kerefall/template/internal/infra/postgres"

var (
	operationRe  = regexp.MustCompile(`(?i)^\s*(select|insert|update|delete|begin|commit|rollback)\b`)
	collectionRe = regexp.MustCompile(`(?i)\b(?:from|into|update)\s+"?([a-z_][a-z0-9_]*)"?`)
)

type queryStartKey struct{}

type queryStart struct {
	at         time.Time
	operation  string
	collection string
}

// queryMetricsTracer пишет метрику db.client.operation.duration и дописывает
// атрибуты db.operation.name и db.collection.name в спан, который создал otelpgx.
type queryMetricsTracer struct {
	duration metric.Float64Histogram
}

func newQueryMetricsTracer() (*queryMetricsTracer, error) {
	duration, err := otel.Meter(meterName).Float64Histogram(
		"db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of database client operations."),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5),
	)
	if err != nil {
		return nil, fmt.Errorf("create db duration histogram: %w", err)
	}

	return &queryMetricsTracer{duration: duration}, nil
}

func (t *queryMetricsTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	operation, collection := parseSQL(data.SQL)

	attrs := []attribute.KeyValue{attribute.String("db.operation.name", operation)}
	if collection != "" {
		attrs = append(attrs, attribute.String("db.collection.name", collection))
	}
	trace.SpanFromContext(ctx).SetAttributes(attrs...)

	return context.WithValue(ctx, queryStartKey{}, queryStart{
		at:         time.Now(),
		operation:  operation,
		collection: collection,
	})
}

func (t *queryMetricsTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	start, ok := ctx.Value(queryStartKey{}).(queryStart)
	if !ok {
		return
	}

	attrs := []attribute.KeyValue{attribute.String("db.operation.name", start.operation)}
	if start.collection != "" {
		attrs = append(attrs, attribute.String("db.collection.name", start.collection))
	}
	if data.Err != nil {
		attrs = append(attrs, attribute.String("error.type", errorType(data.Err)))
	}

	t.duration.Record(ctx, time.Since(start.at).Seconds(), metric.WithAttributes(attrs...))
}

// parseSQL достаёт из запроса тип операции и имя таблицы. Таблиц и операций в
// сервисе мало, поэтому значений у лейблов немного.
func parseSQL(sql string) (operation, collection string) {
	operation = "other"
	if m := operationRe.FindStringSubmatch(sql); m != nil {
		operation = strings.ToUpper(m[1])
	}
	if m := collectionRe.FindStringSubmatch(sql); m != nil {
		collection = strings.ToLower(m[1])
	}

	return operation, collection
}

func errorType(err error) string {
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr):
		return pgErr.Code
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "error"
	}
}
