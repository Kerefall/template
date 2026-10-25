package handler

import (
	"net/http"

	api "github.com/Kerefall/template/internal/generated"
)

// Health - liveness: процесс жив. В бд не ходит
func (h Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
