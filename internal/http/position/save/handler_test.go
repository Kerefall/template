package save_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/domain/trip"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/position/save"
	"github.com/Kerefall/template/internal/model"
)

const validBody = `{
  "coordinates": {"latitude": 59.9350, "longitude": 30.3350},
  "recorded_at": "2026-08-24T12:00:05Z"
}`

func newHandler(t *testing.T) (*save.Handler, *Mockusecase) {
	t.Helper()

	uc := NewMockusecase(gomock.NewController(t))

	return save.New(uc, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil))), uc
}

func post(h *save.Handler, id uuid.UUID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/positions", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Handle(rec, req, id)

	return rec
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var p api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))

	return p.Code
}

func TestHandle_Success(t *testing.T) {
	h, uc := newHandler(t)
	id := uuid.New()
	recordedAt := time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)

	uc.EXPECT().Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in model.SavePositionInput) (position.Position, error) {
			require.Equal(t, id, in.TripID)
			require.Equal(t, 59.935, in.Point.Latitude)
			require.True(t, recordedAt.Equal(in.RecordedAt))
			return position.Position{ID: 42, TripID: id, Point: in.Point, RecordedAt: in.RecordedAt}, nil
		})

	rec := post(h, id, validBody)

	require.Equal(t, http.StatusCreated, rec.Code)

	var body api.TripPosition
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.EqualValues(t, 42, body.Id)
	require.Equal(t, 59.935, body.Coordinates.Latitude)
	require.True(t, recordedAt.Equal(body.RecordedAt))
}

func TestHandle_InvalidBody(t *testing.T) {
	cases := map[string]string{
		"not json":               `not json`,
		"missing recorded_at":    `{"coordinates": {"latitude": 1, "longitude": 2}}`,
		"missing coordinates":    `{"recorded_at": "2026-08-24T12:00:05Z"}`,
		"missing longitude":      `{"coordinates": {"latitude": 1}, "recorded_at": "2026-08-24T12:00:05Z"}`,
		"unparsable recorded_at": `{"coordinates": {"latitude": 1, "longitude": 2}, "recorded_at": "yesterday"}`,
		"unknown field":          `{"coordinates": {"latitude": 1, "longitude": 2}, "recorded_at": "2026-08-24T12:00:05Z", "id": 1}`,
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			// Usecase не должен вызываться: ожидания в моке не заданы.
			h, _ := newHandler(t)

			rec := post(h, uuid.New(), body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "invalid_request", problemCode(t, rec))
		})
	}
}

func TestHandle_UsecaseErrors(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"trip not found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"trip completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{
			"domain validation", &trip.ValidationError{Field: "coordinates.latitude", Reason: "must be between -90 and 90"},
			http.StatusBadRequest, "invalid_request",
		},
		{"repository failure", errors.New("pq: connection refused"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, uc := newHandler(t)
			uc.EXPECT().Save(gomock.Any(), gomock.Any()).Return(position.Position{}, tc.err)

			rec := post(h, uuid.New(), validBody)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantCode, problemCode(t, rec))
			require.NotContains(t, rec.Body.String(), "pq:", "internal error details must not leak")
		})
	}
}
