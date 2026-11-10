// Package get - ручка GET /api/v1/trips/{tripId}.
package get

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/http/domainerror"
	"github.com/Kerefall/template/internal/http/respond"
)

type Handler struct {
	usecase usecase
	timeout time.Duration
	log     *slog.Logger
}

func New(usecase usecase, timeout time.Duration, log *slog.Logger) *Handler {
	return &Handler{usecase: usecase, timeout: timeout, log: log}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, id uuid.UUID) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	t, err := h.usecase.Get(ctx, id)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	respond.JSON(w, http.StatusOK, toResponse(t))
}
