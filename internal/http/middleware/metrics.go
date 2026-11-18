package middleware

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/Kerefall/template/internal/http/middleware"

// Metrics считает http.server.request.duration и http.server.active_requests.
// В лейбл http.route кладётся только шаблон маршрута: если подставлять сюда
// конкретный tripId, на каждую поездку появится своя временная серия, и
// метрика перестанет агрегироваться.
func Metrics(routeOf func(*http.Request) string) (func(http.Handler) http.Handler, error) {
	meter := otel.Meter(meterName)

	duration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests."),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create request duration histogram: %w", err)
	}

	active, err := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of HTTP requests being processed right now."),
	)
	if err != nil {
		return nil, fmt.Errorf("create active requests counter: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			method := attribute.String("http.request.method", r.Method)
			route := attribute.String("http.route", routeOrUnmatched(routeOf(r)))
			start := time.Now()
			sw := newStatusWriter(w)

			active.Add(ctx, 1, metric.WithAttributes(method, route))

			defer finishWith(func(panicked bool) {
				active.Add(ctx, -1, metric.WithAttributes(method, route))
				duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(
					method,
					route,
					attribute.Int("http.response.status_code", sw.status(panicked)),
				))
			})

			next.ServeHTTP(sw, r)
		})
	}, nil
}
