// Package create - ручка POST /api/v1/trips.
package create

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

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

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		domainerror.Write(w, r, h.log, domainerror.InvalidRequest("request body could not be read"))
		return
	}

	in, err := parseInput(body)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	created, err := h.usecase.Create(ctx, in)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+created.ID.String())
	respond.JSON(w, http.StatusCreated, toResponse(created))
}
