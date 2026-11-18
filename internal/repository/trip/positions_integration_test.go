//go:build integration

package trip_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/Kerefall/template/internal/domain/position"
	domain "github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/infra/postgres/transaction"
	"github.com/Kerefall/template/internal/model"
	repo "github.com/Kerefall/template/internal/repository/trip"
)

func micro(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}

// newTripAt создаёт активную поездку нового водителя, начатую startedAt.
func newTripAt(t *testing.T, pool *pgxpool.Pool, startedAt time.Time) domain.Trip {
	t.Helper()

	point := domain.Point{Latitude: 59.9, Longitude: 30.3}
	tr, err := domain.New(uuid.New(), uuid.New(), uuid.New(), point, point, 100, micro(startedAt))
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := pool.Exec(context.Background(), `DELETE FROM trips WHERE id = $1`, tr.ID)
		require.NoError(t, err)
	})

	return tr
}

func contains(trips []model.StaleTrip, id uuid.UUID) bool {
	for _, t := range trips {
		if t.ID == id {
			return true
		}
	}

	return false
}

func TestListStale_SelectsOnlyWhatShouldBePolled(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	ctx := context.Background()
	now := time.Now()

	silent := newTripAt(t, pool, now.Add(-10*time.Minute))        // координат не было, started_at давно
	fresh := newTripAt(t, pool, now.Add(-10*time.Minute))         // координата была только что
	freshStart := newTripAt(t, pool, now.Add(-5*time.Second))     // только начата, координат ещё нет
	finished := newTripAt(t, pool, now.Add(-10*time.Minute))      // завершена
	oldCoordinate := newTripAt(t, pool, now.Add(-10*time.Minute)) // последняя координата давно

	for _, tr := range []domain.Trip{silent, fresh, freshStart, finished, oldCoordinate} {
		require.NoError(t, r.Save(ctx, tr))
	}
	require.NoError(t, r.TouchLastPosition(ctx, fresh.ID, micro(now)))
	require.NoError(t, r.TouchLastPosition(ctx, oldCoordinate.ID, micro(now.Add(-5*time.Minute))))
	require.NoError(t, finished.Finish(micro(now)))
	require.NoError(t, r.Update(ctx, finished))

	// Берём порцию побольше: в базе могут лежать чужие поездки от других запусков.
	got, err := r.ListStale(ctx, time.Minute, 100000, nil)
	require.NoError(t, err)

	require.True(t, contains(got, silent.ID), "поездка без координат и со старым started_at должна попасть")
	require.True(t, contains(got, oldCoordinate.ID), "поездка со старой координатой должна попасть")
	require.False(t, contains(got, fresh.ID), "поездка со свежей координатой не попадает")
	require.False(t, contains(got, freshStart.ID), "только что начатая поездка не попадает")
	require.False(t, contains(got, finished.ID), "завершённая поездка не попадает")

	// Первыми идут те, кто молчит дольше всех: silent (-10 мин) раньше oldCoordinate (-5 мин).
	silentAt, oldAt := -1, -1
	for i, s := range got {
		switch s.ID {
		case silent.ID:
			silentAt = i
		case oldCoordinate.ID:
			oldAt = i
		}
	}
	require.Less(t, silentAt, oldAt)
}

// Порции с одинаковым временем на границе: ни одна поездка не теряется и не читается дважды.
func TestListStale_BatchesDoNotLoseOrDuplicateTripsWithEqualTimestamps(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	ctx := context.Background()
	startedAt := time.Now().Add(-20 * time.Minute)

	want := make(map[uuid.UUID]bool)
	for range 5 {
		tr := newTripAt(t, pool, startedAt) // у всех одинаковое started_at
		require.NoError(t, r.Save(ctx, tr))
		want[tr.ID] = true
	}

	seen := make(map[uuid.UUID]int)
	var cursor *model.StaleCursor
	for {
		batch, err := r.ListStale(ctx, time.Minute, 2, cursor)
		require.NoError(t, err)
		if len(batch) == 0 {
			break
		}
		for _, s := range batch {
			seen[s.ID]++
		}
		last := batch[len(batch)-1]
		cursor = &model.StaleCursor{ActivityAt: last.ActivityAt, ID: last.ID}
	}

	for id := range want {
		require.Equal(t, 1, seen[id], "поездка должна прочитаться ровно один раз")
	}
}

func TestSavePosition_UpdatesLastPositionAt(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	tx := transaction.NewManager(pool)
	ctx := context.Background()
	now := time.Now()

	tr := newTripAt(t, pool, now.Add(-time.Hour))
	require.NoError(t, r.Save(ctx, tr))

	recordedAt := micro(now.Add(-10 * time.Second))
	p, err := position.New(tr.ID, domain.Point{Latitude: 59.9, Longitude: 30.3}, recordedAt, now)
	require.NoError(t, err)

	var saved position.Position
	require.NoError(t, tx.Do(ctx, func(ctx context.Context) error {
		if err := r.TouchLastPosition(ctx, tr.ID, p.RecordedAt); err != nil {
			return err
		}
		saved, err = r.SavePosition(ctx, p)

		return err
	}))
	require.NotZero(t, saved.ID)

	got, err := r.Get(ctx, tr.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastPositionAt)
	require.True(t, recordedAt.Equal(*got.LastPositionAt))
}

// Старая координата, доехавшая позже свежей, не должна состарить поездку.
func TestTouchLastPosition_NeverMovesBackwards(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	ctx := context.Background()
	now := time.Now()

	tr := newTripAt(t, pool, now.Add(-time.Hour))
	require.NoError(t, r.Save(ctx, tr))

	newer := micro(now.Add(-5 * time.Second))
	older := micro(now.Add(-5 * time.Minute))

	require.NoError(t, r.TouchLastPosition(ctx, tr.ID, newer))
	require.NoError(t, r.TouchLastPosition(ctx, tr.ID, older)) // опоздавшая координата

	got, err := r.Get(ctx, tr.ID)
	require.NoError(t, err)
	require.True(t, newer.Equal(*got.LastPositionAt), "last_position_at остался на свежей координате")
}

func TestTouchLastPosition_TripErrors(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	ctx := context.Background()
	now := time.Now()

	require.ErrorIs(t, r.TouchLastPosition(ctx, uuid.New(), micro(now)), domain.ErrTripNotFound)

	tr := newTripAt(t, pool, now.Add(-time.Hour))
	require.NoError(t, r.Save(ctx, tr))
	require.NoError(t, tr.Finish(micro(now)))
	require.NoError(t, r.Update(ctx, tr))

	require.ErrorIs(t, r.TouchLastPosition(ctx, tr.ID, micro(now)), domain.ErrTripCompleted)
}

// Если вставка точки падает, last_position_at откатывается вместе с ней.
func TestSavePosition_RollsBackLastPositionAtOnError(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	tx := transaction.NewManager(pool)
	ctx := context.Background()
	now := time.Now()

	tr := newTripAt(t, pool, now.Add(-time.Hour))
	require.NoError(t, r.Save(ctx, tr))

	// Широта вне диапазона нарушает CHECK в trip_positions: домен её не пропустил бы,
	// но здесь мы специально обходим домен.
	broken := position.Position{
		TripID:     tr.ID,
		Point:      domain.Point{Latitude: 100, Longitude: 30},
		RecordedAt: micro(now),
	}

	err := tx.Do(ctx, func(ctx context.Context) error {
		if err := r.TouchLastPosition(ctx, tr.ID, broken.RecordedAt); err != nil {
			return err
		}
		_, err := r.SavePosition(ctx, broken)

		return err
	})
	require.Error(t, err)

	got, err := r.Get(ctx, tr.ID)
	require.NoError(t, err)
	require.Nil(t, got.LastPositionAt, "last_position_at не должен измениться после отката")
}

func TestListPositions_OrderedByRecordedAtThenID(t *testing.T) {
	pool := newPool(t)
	r := repo.New(pool)
	ctx := context.Background()
	now := time.Now()

	tr := newTripAt(t, pool, now.Add(-time.Hour))
	require.NoError(t, r.Save(ctx, tr))

	t1 := micro(now.Add(-3 * time.Minute))
	t2 := micro(now.Add(-2 * time.Minute))

	// Сохраняем не по порядку: устройство прислало накопленные точки пачкой.
	for _, at := range []time.Time{t2, t1, t2} {
		p, err := position.New(tr.ID, domain.Point{Latitude: 1, Longitude: 2}, at, now)
		require.NoError(t, err)
		_, err = r.SavePosition(ctx, p)
		require.NoError(t, err)
	}

	got, err := r.ListPositions(ctx, tr.ID)
	require.NoError(t, err)
	require.Len(t, got, 3)

	require.True(t, t1.Equal(got[0].RecordedAt))
	require.True(t, t2.Equal(got[1].RecordedAt))
	require.True(t, t2.Equal(got[2].RecordedAt))
	require.Less(t, got[1].ID, got[2].ID, "при равном recorded_at порядок по id")
}
