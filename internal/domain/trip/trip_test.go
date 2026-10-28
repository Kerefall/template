package trip_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Kerefall/template/internal/domain/trip"
)

var now = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func TestNew(t *testing.T) {
	user := uuid.New()
	driver := uuid.New()
	point := trip.Point{Latitude: 59.9398, Longitude: 30.3146}

	cases := []struct {
		name      string
		userID    uuid.UUID
		driverID  uuid.UUID
		start     trip.Point
		end       trip.Point
		price     int64
		wantField string
	}{
		{name: "valid", userID: user, driverID: driver, start: point, end: point, price: 1450},
		{name: "zero price is allowed", userID: user, driverID: driver, start: point, end: point, price: 0},
		{name: "empty user", userID: uuid.Nil, driverID: driver, start: point, end: point, price: 1, wantField: "user_id"},
		{name: "empty driver", userID: user, driverID: uuid.Nil, start: point, end: point, price: 1, wantField: "driver_id"},
		{name: "negative price", userID: user, driverID: driver, start: point, end: point, price: -1, wantField: "price"},
		{
			name: "start latitude out of range", userID: user, driverID: driver,
			start: trip.Point{Latitude: 91}, end: point, price: 1, wantField: "start_point.latitude",
		},
		{
			name: "end longitude out of range", userID: user, driverID: driver,
			start: point, end: trip.Point{Longitude: -181}, price: 1, wantField: "end_point.longitude",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := trip.New(uuid.New(), tc.userID, tc.driverID, tc.start, tc.end, tc.price, now)

			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, trip.StatusActive, got.Status)
				require.Nil(t, got.FinishedAt)
				require.Equal(t, now, got.StartedAt)
				return
			}

			var validationErr *trip.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, tc.wantField, validationErr.Field)
		})
	}
}

func newActiveTrip(t *testing.T) trip.Trip {
	t.Helper()

	point := trip.Point{Latitude: 1, Longitude: 2}
	got, err := trip.New(uuid.New(), uuid.New(), uuid.New(), point, point, 100, now)
	require.NoError(t, err)

	return got
}

func TestFinish(t *testing.T) {
	t.Run("active trip is completed", func(t *testing.T) {
		tr := newActiveTrip(t)
		at := now.Add(time.Hour)

		require.NoError(t, tr.Finish(at))

		require.Equal(t, trip.StatusCompleted, tr.Status)
		require.NotNil(t, tr.FinishedAt)
		require.Equal(t, at, *tr.FinishedAt)
		require.Equal(t, at, tr.UpdatedAt)
	})

	t.Run("completed trip cannot be finished again", func(t *testing.T) {
		tr := newActiveTrip(t)
		first := now.Add(time.Hour)
		require.NoError(t, tr.Finish(first))

		err := tr.Finish(first.Add(time.Hour))

		require.ErrorIs(t, err, trip.ErrTripCompleted)
		require.Equal(t, first, *tr.FinishedAt, "finished_at must not be overwritten")
	})
}
