// Package finish - сценарий «завершить поездку».
package finish

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/model"
)

type UseCase struct {
	repo    tripRepository
	tx      txManager
	metrics metrics
	log     *slog.Logger
	now     func() time.Time
}

func New(repo tripRepository, tx txManager, metrics metrics, log *slog.Logger, now func() time.Time) *UseCase {
	return &UseCase{repo: repo, tx: tx, metrics: metrics, log: log, now: now}
}

// Finish завершает поездку. Строка блокируется на время транзакции, поэтому
// при двух одновременных вызовах второй увидит уже завершённую поездку
// и получит trip.ErrTripCompleted.
func (u *UseCase) Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	var finished trip.Trip

	err := u.tx.Do(ctx, func(ctx context.Context) error {
		t, err := u.repo.GetForUpdate(ctx, id)
		if err != nil {
			return err
		}

		if err := t.Finish(u.now()); err != nil {
			return err
		}

		if err := u.repo.Update(ctx, t); err != nil {
			return err
		}

		if err := u.repo.AddStatusHistory(ctx, model.StatusChange{
			TripID: t.ID,
			From:   trip.StatusActive,
			To:     trip.StatusCompleted,
			Reason: "trip finished",
		}); err != nil {
			return err
		}

		finished = t

		return nil
	})
	if err != nil {
		return trip.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	u.metrics.TripCompleted(ctx)
	u.log.InfoContext(ctx, "trip completed", "trip_id", finished.ID.String())

	return finished, nil
}
