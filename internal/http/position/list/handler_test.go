package list_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/domain/position"
	"github.com/Kerefall/template/internal/domain/trip"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/position/list"
)

func newHandler(t *testing.T) (*list.Handler, *Mockusecase) {
	t.Helper()

	uc := NewMockusecase(gomock.NewController(t))

	return list.New(uc, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil))), uc
}

func get(h *list.Handler, id uuid.UUID) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.Handle(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String()+"/positions", nil), id)

	return rec
}

func TestHandle_ReturnsRouteInTheOrderOfTheUsecase(t *testing.T) {
	h, uc := newHandler(t)
	id := uuid.New()
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	uc.EXPECT().List(gomock.Any(), id).Return([]position.Position{
		{ID: 1, TripID: id, Point: trip.Point{Latitude: 1, Longitude: 2}, RecordedAt: at},
		{ID: 2, TripID: id, Point: trip.Point{Latitude: 3, Longitude: 4}, RecordedAt: at.Add(time.Second)},
	}, nil)

	rec := get(h, id)

	require.Equal(t, http.StatusOK, rec.Code)

	var body api.TripPositionsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Positions, 2)
	require.EqualValues(t, 1, body.Positions[0].Id)
	require.EqualValues(t, 2, body.Positions[1].Id)
}

func TestHandle_EmptyRouteIsAnEmptyArrayNotNull(t *testing.T) {
	h, uc := newHandler(t)
	id := uuid.New()
	uc.EXPECT().List(gomock.Any(), id).Return(nil, nil)

	rec := get(h, id)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"positions": []}`, rec.Body.String())
}

func TestHandle_TripNotFound(t *testing.T) {
	h, uc := newHandler(t)
	id := uuid.New()
	uc.EXPECT().List(gomock.Any(), id).Return(nil, trip.ErrTripNotFound)

	rec := get(h, id)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var body api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "trip_not_found", body.Code)
}
