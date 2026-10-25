package handler

import (
	"context"
	"net/http"

	api "github.com/Kerefall/template/internal/generated"
)

// Ready - readiness: сервис готов принимать трафик, если доступна бд
func (h Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.QueryTimeout)
	defer cancel()

	if err := h.DB.Ping(ctx); err != nil {
		h.Log.Warn("readiness check failed", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}

	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
