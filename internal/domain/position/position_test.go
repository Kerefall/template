package position_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/domain/trip"
)

var now = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func TestNew(t *testing.T) {
	ok := trip.Point{Latitude: 59.935, Longitude: 30.335}

	cases := []struct {
		name       string
		point      trip.Point
		recordedAt time.Time
		wantField  string
	}{
		{name: "valid", point: ok, recordedAt: now.Add(-time.Hour)},
		{name: "slightly in the future is allowed", point: ok, recordedAt: now.Add(30 * time.Second)},
		{name: "too far in the future", point: ok, recordedAt: now.Add(2 * time.Minute), wantField: "recorded_at"},
		{name: "latitude out of range", point: trip.Point{Latitude: 91}, recordedAt: now, wantField: "coordinates.latitude"},
		{name: "longitude out of range", point: trip.Point{Longitude: -181}, recordedAt: now, wantField: "coordinates.longitude"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := position.New(uuid.New(), tc.point, tc.recordedAt, now)

			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, tc.recordedAt.UTC(), got.RecordedAt)
				return
			}

			var validationErr *trip.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, tc.wantField, validationErr.Field)
		})
	}
}
