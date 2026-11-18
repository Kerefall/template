package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Logging пишет одну запись на запрос, когда ответ готов. Тела запроса и ответа
// не логируются. Уровень Info для 2xx и 4xx, Error только для 5xx.
// trace_id и span_id добавляет сам логгер из контекста запроса.
func Logging(log *slog.Logger, routeOf func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			route := routeOrUnmatched(routeOf(r))
			sw := newStatusWriter(w)

			defer finishWith(func(panicked bool) {
				status := sw.status(panicked)

				level := slog.LevelInfo
				if status >= http.StatusInternalServerError {
					level = slog.LevelError
				}

				log.LogAttrs(r.Context(), level, "request handled",
					slog.String("method", r.Method),
					slog.String("route", route),
					slog.Int("status", status),
					slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				)
			})

			next.ServeHTTP(sw, r)
		})
	}
}
