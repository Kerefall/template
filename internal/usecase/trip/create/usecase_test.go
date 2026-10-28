package create_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/model"
	"github.com/Kerefall/template/internal/usecase/trip/create"
)

var fixedNow = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

type fixture struct {
	repo    *MocktripRepository
	tx      *MocktxManager
	metrics *Mockmetrics
	uc      *create.UseCase
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	f := fixture{
		repo:    NewMocktripRepository(ctrl),
		tx:      NewMocktxManager(ctrl),
		metrics: NewMockmetrics(ctrl),
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	f.uc = create.New(f.repo, f.tx, f.metrics, log, func() time.Time { return fixedNow })

	return f
}

// runInTx заставляет мок менеджера транзакций просто вызвать переданную функцию.
func (f fixture) runInTx() {
	f.tx.EXPECT().
		Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error {
			return fn(ctx)
		})
}

func validInput() model.CreateTripInput {
	return model.CreateTripInput{
		UserID:   uuid.New(),
		DriverID: uuid.New(),
		Start:    trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:      trip.Point{Latitude: 59.929, Longitude: 30.3626},
		Price:    1450,
	}
}

func TestCreate_Success(t *testing.T) {
	f := newFixture(t)
	in := validInput()

	var saved trip.Trip
	f.runInTx()
	f.repo.EXPECT().Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, tr trip.Trip) error {
			saved = tr
			return nil
		})
	f.repo.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, change model.StatusChange) error {
			require.Equal(t, saved.ID, change.TripID)
			require.Empty(t, change.From)
			require.Equal(t, trip.StatusActive, change.To)
			return nil
		})
	f.metrics.EXPECT().TripCreated(gomock.Any())

	got, err := f.uc.Create(context.Background(), in)

	require.NoError(t, err)
	require.Equal(t, saved, got)
	require.NotEqual(t, uuid.Nil, got.ID)
	require.Equal(t, trip.StatusActive, got.Status)
	require.Equal(t, in.DriverID, got.DriverID)
	require.Equal(t, fixedNow, got.StartedAt)
}

func TestCreate_DriverBusy(t *testing.T) {
	f := newFixture(t)

	f.runInTx()
	f.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.ErrDriverBusy)
	// История не пишется, метрика не растёт.

	_, err := f.uc.Create(context.Background(), validInput())

	require.ErrorIs(t, err, trip.ErrDriverBusy)
}

func TestCreate_InvalidInput(t *testing.T) {
	f := newFixture(t)
	in := validInput()
	in.Price = -1

	// Ни транзакция, ни репозиторий не вызываются: ожидания в моках не заданы.
	_, err := f.uc.Create(context.Background(), in)

	var validationErr *trip.ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, "price", validationErr.Field)
}

func TestCreate_HistoryErrorFailsWholeTx(t *testing.T) {
	f := newFixture(t)
	boom := errors.New("insert history failed")

	f.runInTx()
	f.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	f.repo.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).Return(boom)

	_, err := f.uc.Create(context.Background(), validInput())

	require.ErrorIs(t, err, boom)
}
