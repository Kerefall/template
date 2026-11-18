// Package ready - ручка GET /ready (readiness): сервис готов, если доступна бд.
package ready

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/respond"
)

type Handler struct {
	db      pinger
	timeout time.Duration
	log     *slog.Logger
}

func New(db pinger, timeout time.Duration, log *slog.Logger) *Handler {
	return &Handler{db: db, timeout: timeout, log: log}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.log.WarnContext(ctx, "readiness check failed", "error", err)
		respond.JSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})

		return
	}

	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
