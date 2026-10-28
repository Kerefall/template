package middleware

import (
	"net/http"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/Kerefall/template/internal/http/middleware"

// Tracing создаёт корневой спан на входящий запрос. Имя спана - метод и
// шаблон маршрута (POST /api/v1/trips), а не полный URL. Контекст трейса
// из заголовков W3C TraceContext продолжается, если клиент его прислал.
func Tracing(routeOf func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			route := routeOrUnmatched(routeOf(r))

			ctx, span := otel.Tracer(tracerName).Start(ctx, r.Method+" "+route,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					attribute.String("http.request.method", r.Method),
					attribute.String("http.route", route),
				),
			)

			sw := newStatusWriter(w)
			defer finishWith(func(panicked bool) {
				status := sw.status(panicked)

				span.SetAttributes(attribute.Int("http.response.status_code", status))
				if status >= http.StatusInternalServerError {
					span.SetStatus(codes.Error, http.StatusText(status))
					span.SetAttributes(attribute.String("error.type", strconv.Itoa(status)))
				}
				span.End()
			})

			next.ServeHTTP(sw, r.WithContext(ctx))
		})
	}
}
