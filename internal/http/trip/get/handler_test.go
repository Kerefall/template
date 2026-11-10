package get_test

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

	"github.com/Kerefall/template/internal/domain/trip"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/trip/get"
)

func newHandler(t *testing.T) (*get.Handler, *Mockusecase) {
	t.Helper()

	uc := NewMockusecase(gomock.NewController(t))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return get.New(uc, time.Second, log), uc
}

func TestHandle_Success(t *testing.T) {
	h, uc := newHandler(t)
	id := uuid.New()
	uc.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{ID: id, Status: trip.StatusActive, Price: 100}, nil)

	rec := httptest.NewRecorder()
	h.Handle(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil), id)

	require.Equal(t, http.StatusOK, rec.Code)

	var body api.Trip
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, int64(100), body.Price)
}

func TestHandle_NotFound(t *testing.T) {
	h, uc := newHandler(t)
	id := uuid.New()
	uc.EXPECT().Get(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)

	rec := httptest.NewRecorder()
	h.Handle(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil), id)

	require.Equal(t, http.StatusNotFound, rec.Code)

	var body api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "trip_not_found", body.Code)
}
