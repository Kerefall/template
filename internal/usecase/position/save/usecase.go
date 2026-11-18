// Package save - сценарий «сохранить координату поездки».
package save

import (
	"context"
	"fmt"
	"time"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/model"
)

type UseCase struct {
	repo    positionRepository
	tx      txManager
	metrics metrics
	queue   staleQueue
	now     func() time.Time
}

func New(repo positionRepository, tx txManager, metrics metrics, queue staleQueue, now func() time.Time) *UseCase {
	return &UseCase{repo: repo, tx: tx, metrics: metrics, queue: queue, now: now}
}

// Save сохраняет точку маршрута и двигает last_position_at в одной транзакции.
// Сначала обновляется поездка: UPDATE ... WHERE status = 'active' берёт блокировку
// строки, поэтому одновременное завершение поездки не даст сохранить координату
// в уже завершённую поездку.
func (u *UseCase) Save(ctx context.Context, in model.SavePositionInput) (position.Position, error) {
	p, err := position.New(in.TripID, in.Point, in.RecordedAt, u.now())
	if err != nil {
		return position.Position{}, err
	}

	var saved position.Position
	err = u.tx.Do(ctx, func(ctx context.Context) error {
		if err := u.repo.TouchLastPosition(ctx, p.TripID, p.RecordedAt); err != nil {
			return err
		}

		var saveErr error
		saved, saveErr = u.repo.SavePosition(ctx, p)

		return saveErr
	})
	if err != nil {
		return position.Position{}, fmt.Errorf("save position: %w", err)
	}

	u.metrics.PositionSaved(ctx)
	// Координата пришла: опрашивать устройство по этой поездке уже не нужно.
	u.queue.Cancel(saved.TripID)

	return saved, nil
}
