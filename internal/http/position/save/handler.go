// Package save - ручка POST /api/v1/trips/{tripId}/positions.
package save

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/http/domainerror"
	"github.com/Kerefall/template/internal/http/respond"
)

const maxBodyBytes = 1 << 20

type Handler struct {
	usecase usecase
	timeout time.Duration
	log     *slog.Logger
}

func New(usecase usecase, timeout time.Duration, log *slog.Logger) *Handler {
	return &Handler{usecase: usecase, timeout: timeout, log: log}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, tripID uuid.UUID) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		domainerror.Write(w, r, h.log, domainerror.InvalidRequest("request body could not be read"))
		return
	}

	in, err := parseInput(tripID, body)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	saved, err := h.usecase.Save(ctx, in)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	respond.JSON(w, http.StatusCreated, toResponse(saved))
}
