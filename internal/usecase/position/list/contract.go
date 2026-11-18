//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/domain/trip"
)

type (
	tripRepository interface {
		Get(ctx context.Context, id uuid.UUID) (trip.Trip, error)
	}

	positionRepository interface {
		ListPositions(ctx context.Context, tripID uuid.UUID) ([]position.Position, error)
	}
)
