//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	domain "github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/infra/postgres/transaction"
	"github.com/Kerefall/template/internal/model"
	repo "github.com/Kerefall/template/internal/repository/trip"
)

// Тесты работают с базой из tripgoctl environment start (DATABASE_URL) и
// ожидают накатанные миграции: make migrate.

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	return pool
}

// newTrip создаёт поездку нового водителя и обещает убрать её из базы после теста.
func newTrip(t *testing.T, pool *pgxpool.Pool) domain.Trip {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Microsecond)
	point := domain.Point{Latitude: 59.9398, Longitude: 30.3146}

	tr, err := domain.New(uuid.New(), uuid.New(), uuid.New(), point, point, 1450, now)
	require.NoError(t, err)

	t.Cleanup(func() {
		// История удаляется каскадом.
		_, err := pool.Exec(context.Background(), `DELETE FROM trips WHERE id = $1`, tr.ID)
		require.NoError(t, err)
	})

	return tr
}

func TestSaveAndGet(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	ctx := context.Background()
	want := newTrip(t, pool)

	require.NoError(t, r.Save(ctx, want))

	got, err := r.Get(ctx, want.ID)
	require.NoError(t, err)
	require.Equal(t, want.ID, got.ID)
	require.Equal(t, want.UserID, got.UserID)
	require.Equal(t, want.DriverID, got.DriverID)
	require.Equal(t, want.Start, got.Start)
	require.Equal(t, want.End, got.End)
	require.Equal(t, want.Price, got.Price)
	require.Equal(t, domain.StatusActive, got.Status)
	require.True(t, want.StartedAt.Equal(got.StartedAt))
	require.Nil(t, got.FinishedAt)
}

func TestGetNotFound(t *testing.T) {
	r := repo.New(newPool(t))

	_, err := r.Get(context.Background(), uuid.New())

	require.ErrorIs(t, err, domain.ErrTripNotFound)
}

func TestSaveDriverBusy(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	ctx := context.Background()

	first := newTrip(t, pool)
	require.NoError(t, r.Save(ctx, first))

	second := newTrip(t, pool)
	second.DriverID = first.DriverID

	require.ErrorIs(t, r.Save(ctx, second), domain.ErrDriverBusy)

	// После завершения первой поездки водитель снова свободен.
	require.NoError(t, first.Finish(time.Now().UTC().Truncate(time.Microsecond)))
	require.NoError(t, r.Update(ctx, first))
	require.NoError(t, r.Save(ctx, second))
}

func TestFinishFlow(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	tx := transaction.NewManager(pool)
	ctx := context.Background()

	tr := newTrip(t, pool)
	require.NoError(t, r.Save(ctx, tr))

	at := time.Now().UTC().Truncate(time.Microsecond)
	err := tx.Do(ctx, func(ctx context.Context) error {
		locked, err := r.GetForUpdate(ctx, tr.ID)
		if err != nil {
			return err
		}
		if err := locked.Finish(at); err != nil {
			return err
		}
		if err := r.Update(ctx, locked); err != nil {
			return err
		}

		return r.AddStatusHistory(ctx, model.StatusChange{
			TripID: tr.ID, From: domain.StatusActive, To: domain.StatusCompleted, Reason: "trip finished",
		})
	})
	require.NoError(t, err)

	got, err := r.Get(ctx, tr.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, got.Status)
	require.NotNil(t, got.FinishedAt)
	require.True(t, at.Equal(*got.FinishedAt))
}

// Обе вставки (trips и trip_status_history) идут в одной транзакции:
// если вторая падает, первая откатывается.
func TestTransactionRollsBackBothInserts(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	tx := transaction.NewManager(pool)
	ctx := context.Background()
	tr := newTrip(t, pool)

	err := tx.Do(ctx, func(ctx context.Context) error {
		if err := r.Save(ctx, tr); err != nil {
			return err
		}

		// Статус вне допустимых значений нарушает CHECK в trip_status_history.
		return r.AddStatusHistory(ctx, model.StatusChange{
			TripID: tr.ID, To: domain.Status("bogus"), Reason: "must fail",
		})
	})
	require.Error(t, err)
	require.False(t, errors.Is(err, domain.ErrDriverBusy))

	_, err = r.Get(ctx, tr.ID)
	require.ErrorIs(t, err, domain.ErrTripNotFound, "trip must not stay in the database")
}
