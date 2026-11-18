// Package health - ручка GET /health (liveness): процесс жив, в бд не ходит.
package health

import (
	"net/http"

	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/respond"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Handle(w http.ResponseWriter, _ *http.Request) {
	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
