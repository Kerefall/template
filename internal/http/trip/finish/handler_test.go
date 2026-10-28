package finish_test

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
	"github.com/Kerefall/template/internal/http/trip/finish"
)

func newHandler(t *testing.T) (*finish.Handler, *Mockusecase) {
	t.Helper()

	uc := NewMockusecase(gomock.NewController(t))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return finish.New(uc, time.Second, log), uc
}

func serve(h *finish.Handler, id uuid.UUID) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.Handle(rec, httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil), id)

	return rec
}

func TestHandle_Success(t *testing.T) {
	h, uc := newHandler(t)
	id := uuid.New()
	at := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	uc.EXPECT().Finish(gomock.Any(), id).Return(trip.Trip{ID: id, Status: trip.StatusCompleted, FinishedAt: &at}, nil)

	rec := serve(h, id)

	require.Equal(t, http.StatusOK, rec.Code)

	var body api.Trip
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, api.TripStatus("completed"), body.Status)
	require.NotNil(t, body.FinishedAt)
}

func TestHandle_Errors(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"already completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{"not found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, uc := newHandler(t)
			id := uuid.New()
			uc.EXPECT().Finish(gomock.Any(), id).Return(trip.Trip{}, tc.err)

			rec := serve(h, id)

			require.Equal(t, tc.wantStatus, rec.Code)

			var body api.Problem
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tc.wantCode, body.Code)
		})
	}
}
