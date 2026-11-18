// Package list - сценарий «получить маршрут поездки».
package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/position"
)

type UseCase struct {
	trips     tripRepository
	positions positionRepository
}

func New(trips tripRepository, positions positionRepository) *UseCase {
	return &UseCase{trips: trips, positions: positions}
}

// List возвращает все точки маршрута по recorded_at, затем по id. Если поездки нет,
// возвращает trip.ErrTripNotFound; у существующей поездки без координат маршрут пуст.
func (u *UseCase) List(ctx context.Context, tripID uuid.UUID) ([]position.Position, error) {
	if _, err := u.trips.Get(ctx, tripID); err != nil {
		return nil, err
	}

	return u.positions.ListPositions(ctx, tripID)
}
