package save_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/model"
	"github.com/Kerefall/template/internal/usecase/position/save"
)

var fixedNow = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

type fixture struct {
	repo    *MockpositionRepository
	tx      *MocktxManager
	metrics *Mockmetrics
	queue   *MockstaleQueue
	uc      *save.UseCase
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	f := fixture{
		repo:    NewMockpositionRepository(ctrl),
		tx:      NewMocktxManager(ctrl),
		metrics: NewMockmetrics(ctrl),
		queue:   NewMockstaleQueue(ctrl),
	}
	f.uc = save.New(f.repo, f.tx, f.metrics, f.queue, func() time.Time { return fixedNow })

	return f
}

func (f fixture) runInTx() {
	f.tx.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})
}

func input() model.SavePositionInput {
	return model.SavePositionInput{
		TripID:     uuid.New(),
		Point:      trip.Point{Latitude: 59.935, Longitude: 30.335},
		RecordedAt: fixedNow.Add(-5 * time.Second),
	}
}

func TestSave_Success(t *testing.T) {
	f := newFixture(t)
	in := input()

	f.runInTx()
	f.repo.EXPECT().TouchLastPosition(gomock.Any(), in.TripID, in.RecordedAt).Return(nil)
	f.repo.EXPECT().SavePosition(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p position.Position) (position.Position, error) {
			require.Equal(t, in.TripID, p.TripID)
			require.Equal(t, in.Point, p.Point)
			p.ID = 42
			return p, nil
		})
	f.metrics.EXPECT().PositionSaved(gomock.Any())
	f.queue.EXPECT().Cancel(in.TripID) // по поездке пришла координата: опрос не нужен

	got, err := f.uc.Save(context.Background(), in)

	require.NoError(t, err)
	require.EqualValues(t, 42, got.ID)
	require.Equal(t, in.RecordedAt, got.RecordedAt)
}

func TestSave_TripErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"trip not found", trip.ErrTripNotFound},
		{"trip completed", trip.ErrTripCompleted},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			in := input()

			f.runInTx()
			f.repo.EXPECT().TouchLastPosition(gomock.Any(), in.TripID, in.RecordedAt).Return(tc.err)
			// Точка не сохраняется, метрика не растёт, очередь не трогаем.

			_, err := f.uc.Save(context.Background(), in)

			require.ErrorIs(t, err, tc.err)
		})
	}
}

func TestSave_InvalidInputNeverReachesTheRepository(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*model.SavePositionInput)
		wantField string
	}{
		{"latitude out of range", func(in *model.SavePositionInput) { in.Point.Latitude = 91 }, "coordinates.latitude"},
		{"recorded_at in the future", func(in *model.SavePositionInput) { in.RecordedAt = fixedNow.Add(5 * time.Minute) }, "recorded_at"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			in := input()
			tc.mutate(&in)

			_, err := f.uc.Save(context.Background(), in)

			var validationErr *trip.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, tc.wantField, validationErr.Field)
		})
	}
}

func TestSave_InsertErrorFailsTheWholeTransaction(t *testing.T) {
	f := newFixture(t)
	in := input()
	boom := errors.New("insert failed")

	f.runInTx()
	f.repo.EXPECT().TouchLastPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	f.repo.EXPECT().SavePosition(gomock.Any(), gomock.Any()).Return(position.Position{}, boom)

	_, err := f.uc.Save(context.Background(), in)

	require.ErrorIs(t, err, boom)
}
