package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/Kerefall/template/internal/http/domainerror"
)

// Recover ловит панику хендлера, пишет её в лог со стектрейсом и отдаёт 500.
// Процесс при этом не падает.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// Так net/http просит прервать ответ молча.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				log.ErrorContext(r.Context(), "panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprint(rec),
					"stack", string(debug.Stack()),
				)

				domainerror.Write(w, r, nil, errors.New("panic in handler"))
			}()

			next.ServeHTTP(w, r)
		})
	}
}
