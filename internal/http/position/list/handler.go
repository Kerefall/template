// Package list - ручка GET /api/v1/trips/{tripId}/positions.
package list

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/position"
	api "github.com/Kerefall/template/internal/generated"
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

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, tripID uuid.UUID) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	positions, err := h.usecase.List(ctx, tripID)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	respond.JSON(w, http.StatusOK, toResponse(positions))
}

func toResponse(positions []position.Position) api.TripPositionsResponse {
	items := make([]api.TripPosition, 0, len(positions))
	for _, p := range positions {
		items = append(items, api.TripPosition{
			Id:          p.ID,
			TripId:      p.TripID,
			Coordinates: api.Coordinates{Latitude: p.Point.Latitude, Longitude: p.Point.Longitude},
			RecordedAt:  p.RecordedAt,
		})
	}

	return api.TripPositionsResponse{Positions: items}
}
