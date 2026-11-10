// Package middleware - сквозная обработка HTTP-запросов.
// Порядок подключения: Recover -> Tracing -> Logging -> Metrics -> роутер.
package middleware

import "net/http"

// statusWriter запоминает статус ответа, чтобы middleware могли его записать
// в спан, лог и метрику.
type statusWriter struct {
	http.ResponseWriter
	code int
}

// newStatusWriter не оборачивает writer повторно: все middleware цепочки
// видят один и тот же статус.
func newStatusWriter(w http.ResponseWriter) *statusWriter {
	if sw, ok := w.(*statusWriter); ok {
		return sw
	}

	return &statusWriter{ResponseWriter: w}
}

func (w *statusWriter) WriteHeader(code int) {
	if w.code == 0 {
		w.code = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.code == 0 {
		w.code = http.StatusOK
	}

	return w.ResponseWriter.Write(b)
}

// Unwrap нужен http.ResponseController.
func (w *statusWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// status возвращает итоговый статус. Если хендлер упал с паникой, ответ ещё
// не отправлен и его напишет Recover, поэтому считаем это 500.
func (w *statusWriter) status(panicked bool) int {
	switch {
	case panicked:
		return http.StatusInternalServerError
	case w.code == 0:
		return http.StatusOK
	default:
		return w.code
	}
}

// finishWith вызывается через defer. Он даёт middleware записать итог
// запроса, в том числе при панике, и пробрасывает панику дальше к Recover.
func finishWith(finish func(panicked bool)) {
	rec := recover()
	finish(rec != nil)

	if rec != nil {
		panic(rec)
	}
}

const unmatchedRoute = "unmatched"

// routeOrUnmatched подставляет понятное значение, если путь не совпал ни с одним маршрутом.
func routeOrUnmatched(route string) string {
	if route == "" {
		return unmatchedRoute
	}

	return route
}
