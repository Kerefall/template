package finish_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/model"
	"github.com/Kerefall/template/internal/usecase/trip/finish"
)

var fixedNow = time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)

type fixture struct {
	repo    *MocktripRepository
	tx      *MocktxManager
	metrics *Mockmetrics
	uc      *finish.UseCase
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	f := fixture{
		repo:    NewMocktripRepository(ctrl),
		tx:      NewMocktxManager(ctrl),
		metrics: NewMockmetrics(ctrl),
	}

	f.tx.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.uc = finish.New(f.repo, f.tx, f.metrics, log, func() time.Time { return fixedNow })

	return f
}

func TestFinish_ActiveTrip(t *testing.T) {
	f := newFixture(t)
	id := uuid.New()
	active := trip.Trip{ID: id, Status: trip.StatusActive}

	f.repo.EXPECT().GetForUpdate(gomock.Any(), id).Return(active, nil)
	f.repo.EXPECT().Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, tr trip.Trip) error {
			require.Equal(t, trip.StatusCompleted, tr.Status)
			require.NotNil(t, tr.FinishedAt)
			require.Equal(t, fixedNow, *tr.FinishedAt)
			return nil
		})
	f.repo.EXPECT().AddStatusHistory(gomock.Any(), model.StatusChange{
		TripID: id,
		From:   trip.StatusActive,
		To:     trip.StatusCompleted,
		Reason: "trip finished",
	}).Return(nil)
	f.metrics.EXPECT().TripCompleted(gomock.Any())

	got, err := f.uc.Finish(context.Background(), id)

	require.NoError(t, err)
	require.Equal(t, trip.StatusCompleted, got.Status)
	require.Equal(t, fixedNow, *got.FinishedAt)
}

func TestFinish_AlreadyCompleted(t *testing.T) {
	f := newFixture(t)
	id := uuid.New()
	done := fixedNow.Add(-time.Hour)
	completed := trip.Trip{ID: id, Status: trip.StatusCompleted, FinishedAt: &done}

	f.repo.EXPECT().GetForUpdate(gomock.Any(), id).Return(completed, nil)
	// Update, история и метрика не вызываются: ожидания не заданы.

	_, err := f.uc.Finish(context.Background(), id)

	require.ErrorIs(t, err, trip.ErrTripCompleted)
}

func TestFinish_NotFound(t *testing.T) {
	f := newFixture(t)
	id := uuid.New()

	f.repo.EXPECT().GetForUpdate(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)

	_, err := f.uc.Finish(context.Background(), id)

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}
