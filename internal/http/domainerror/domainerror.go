// Package domainerror - единственное место, где доменные ошибки
// превращаются в HTTP-ответ (RFC 9457, application/problem+json).
package domainerror

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Kerefall/template/internal/domain/trip"
	api "github.com/Kerefall/template/internal/generated"
)

const typeBase = "https://tripgo.example/problems/"

var (
	// ErrRouteNotFound - запрошен путь, которого нет.
	ErrRouteNotFound = errors.New("route not found")
	// ErrMethodNotAllowed - у пути нет такого метода.
	ErrMethodNotAllowed = errors.New("method not allowed")
)

type invalidRequestError struct {
	detail string
}

func (e invalidRequestError) Error() string {
	return e.detail
}

// InvalidRequest - ошибка разбора запроса на транспортном уровне
// (битый JSON, не хватает поля, неверный параметр пути).
func InvalidRequest(detail string) error {
	return invalidRequestError{detail: detail}
}

// Problem выбирает HTTP-статус и тело ответа для ошибки. Всё незнакомое
// превращается в 500 без подробностей: внутренние детали клиенту не отдаём.
func Problem(err error) (int, api.Problem) {
	var (
		invalidRequest invalidRequestError
		validation     *trip.ValidationError
	)

	switch {
	case errors.As(err, &invalidRequest):
		return problem(http.StatusBadRequest, "invalid-request", "Invalid request", "invalid_request", invalidRequest.detail)
	case errors.As(err, &validation):
		return problem(http.StatusBadRequest, "invalid-request", "Invalid request", "invalid_request", validation.Error())
	case errors.Is(err, trip.ErrTripNotFound):
		return problem(http.StatusNotFound, "trip-not-found", "Trip not found", "trip_not_found", "Trip was not found")
	case errors.Is(err, trip.ErrTripCompleted):
		return problem(http.StatusConflict, "trip-completed", "Trip completed", "trip_completed", "Operation is not allowed for a completed trip")
	case errors.Is(err, trip.ErrDriverBusy):
		return problem(http.StatusConflict, "driver-busy", "Driver busy", "driver_busy", "Driver already has an active trip")
	case errors.Is(err, ErrRouteNotFound):
		return problem(http.StatusNotFound, "not-found", "Not found", "not_found", "Resource was not found")
	case errors.Is(err, ErrMethodNotAllowed):
		return problem(http.StatusMethodNotAllowed, "method-not-allowed", "Method not allowed", "method_not_allowed", "Method is not allowed for this resource")
	default:
		return problem(http.StatusInternalServerError, "internal-error", "Internal Server Error", "internal_error", "Internal server error")
	}
}

func problem(status int, slug, title, code, detail string) (int, api.Problem) {
	return status, api.Problem{
		Type:   typeBase + slug,
		Title:  title,
		Status: int32(status), //nolint:gosec // статус HTTP всегда помещается в int32
		Code:   code,
		Detail: &detail,
	}
}

// Write пишет ответ-ошибку. Для 5xx причина попадает в лог (если log не nil)
// и в спан запроса, а клиент получает только общий текст.
func Write(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	status, body := Problem(err)

	instance := r.URL.Path
	body.Instance = &instance

	if status >= http.StatusInternalServerError {
		if log != nil {
			log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		}

		span := trace.SpanFromContext(r.Context())
		span.RecordError(err)
		span.SetStatus(codes.Error, "internal error")
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
