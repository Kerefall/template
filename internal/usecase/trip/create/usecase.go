// Package create - сценарий «создать поездку».
package create

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

// Create создаёт активную поездку. Поездка и запись в журнале статусов
// сохраняются в одной транзакции.
func (u *UseCase) Create(ctx context.Context, in model.CreateTripInput) (trip.Trip, error) {
	t, err := trip.New(uuid.New(), in.UserID, in.DriverID, in.Start, in.End, in.Price, u.now())
	if err != nil {
		return trip.Trip{}, err
	}

	err = u.tx.Do(ctx, func(ctx context.Context) error {
		if err := u.repo.Save(ctx, t); err != nil {
			return err
		}

		return u.repo.AddStatusHistory(ctx, model.StatusChange{
			TripID: t.ID,
			To:     trip.StatusActive,
			Reason: "trip created",
		})
	})
	if err != nil {
		return trip.Trip{}, fmt.Errorf("create trip: %w", err)
	}

	u.metrics.TripCreated(ctx)
	u.log.InfoContext(ctx, "trip created", "trip_id", t.ID.String(), "driver_id", t.DriverID.String())

	return t, nil
}
