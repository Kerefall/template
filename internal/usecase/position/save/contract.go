//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package save

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/position"
)

type (
	positionRepository interface {
		TouchLastPosition(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error
		SavePosition(ctx context.Context, p position.Position) (position.Position, error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	metrics interface {
		PositionSaved(ctx context.Context)
	}

	// staleQueue убирает поездку из очереди опроса, когда по ней пришла координата.
	staleQueue interface {
		Cancel(tripID uuid.UUID)
	}
)
