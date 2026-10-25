package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/Kerefall/template/internal/domain"
	api "github.com/Kerefall/template/internal/generated"
)

const problemTypeBase = "https://tripgo.example/problems/"

// writeProblem отдаёт ошибку по RFC 9457 в application/problem+json
func writeProblem(w http.ResponseWriter, r *http.Request, status int, slug, title, code, detail string) {
	instance := r.URL.Path

	problem := api.Problem{
		Type:     problemTypeBase + slug,
		Title:    title,
		Status:   int32(status),
		Detail:   &detail,
		Instance: &instance,
		Code:     code,
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}

func writeInvalidRequest(w http.ResponseWriter, r *http.Request, detail string) {
	writeProblem(w, r, http.StatusBadRequest, "invalid-request", "Invalid request", "invalid_request", detail)
}

// BindErrorHandler вызывается сгенерированным кодом, когда не удалось разобрать
// параметры запроса (например, tripId не UUID)
func BindErrorHandler(w http.ResponseWriter, r *http.Request, err error) {
	detail := "Request validation failed"

	var formatErr *api.InvalidParamFormatError
	if errors.As(err, &formatErr) {
		detail = fmt.Sprintf("Invalid value of parameter %q", formatErr.ParamName)
	}

	writeInvalidRequest(w, r, detail)
}

// writeError - единственное место, где доменные ошибки превращаются в HTTP-ответ
// Всё незнакомое уходит клиенту как 500 без подробностей, подробности пишем в лог
func (h Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrTripNotFound):
		writeProblem(w, r, http.StatusNotFound, "trip-not-found", "Trip not found", "trip_not_found", "Trip was not found")
	case errors.Is(err, domain.ErrTripCompleted):
		writeProblem(w, r, http.StatusConflict, "trip-completed", "Trip completed", "trip_completed", "Operation is not allowed for a completed trip")
	case errors.Is(err, domain.ErrDriverBusy):
		writeProblem(w, r, http.StatusConflict, "driver-busy", "Driver busy", "driver_busy", "Driver already has an active trip")
	default:
		h.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "internal-error", "Internal Server Error", "internal_error", "Internal server error")
	}
}

// NotFound и MethodNotAllowed - ответы роутера на неизвестный путь и метод
// Отдаём их в том же problem+json, что и остальные ошибки, а не обычным текстом
func NotFound(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, http.StatusNotFound, "not-found", "Not found", "not_found", "Resource was not found")
}

func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, http.StatusMethodNotAllowed, "method-not-allowed", "Method not allowed", "method_not_allowed", "Method is not allowed for this resource")
}
