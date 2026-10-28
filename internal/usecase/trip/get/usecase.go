// Package get - сценарий «получить поездку».
package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/trip"
)

type UseCase struct {
	repo tripRepository
}

func New(repo tripRepository) *UseCase {
	return &UseCase{repo: repo}
}

// Get возвращает поездку или trip.ErrTripNotFound.
func (u *UseCase) Get(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	return u.repo.Get(ctx, id)
}
