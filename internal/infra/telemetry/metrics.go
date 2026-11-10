package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/Kerefall/template/internal/infra/telemetry"

// Metrics - бизнес-метрики сервиса. Реализует интерфейсы metrics
// в пакетах usecase, поэтому usecase про OpenTelemetry ничего не знает.
type Metrics struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func newMetrics() (*Metrics, error) {
	meter := otel.Meter(meterName)

	created, err := meter.Int64Counter(
		"tripgo.trips.created",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Created trips."),
	)
	if err != nil {
		return nil, fmt.Errorf("create trips.created counter: %w", err)
	}

	completed, err := meter.Int64Counter(
		"tripgo.trips.completed",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Completed trips."),
	)
	if err != nil {
		return nil, fmt.Errorf("create trips.completed counter: %w", err)
	}

	return &Metrics{created: created, completed: completed}, nil
}

func (m *Metrics) TripCreated(ctx context.Context) {
	m.created.Add(ctx, 1)
}

func (m *Metrics) TripCompleted(ctx context.Context) {
	m.completed.Add(ctx, 1)
}
