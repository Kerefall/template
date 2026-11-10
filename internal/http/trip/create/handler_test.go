package create_test

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

	"github.com/Kerefall/template/internal/domain/trip"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/trip/create"
	"github.com/Kerefall/template/internal/model"
)

const validBody = `{
  "user_id": "5cb72c04-7650-45c9-a79b-bcdba0631e0c",
  "driver_id": "8860b315-ec86-42eb-a17c-7c163d721ff5",
  "start_point": {"latitude": 59.9398, "longitude": 30.3146},
  "end_point": {"latitude": 59.929, "longitude": 30.3626},
  "price": 1450
}`

func newHandler(t *testing.T) (*create.Handler, *Mockusecase) {
	t.Helper()

	uc := NewMockusecase(gomock.NewController(t))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	return create.New(uc, time.Second, log), uc
}

func post(h *create.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Handle(rec, req)

	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) api.Problem {
	t.Helper()

	var p api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))

	return p
}

func TestHandle_Success(t *testing.T) {
	h, uc := newHandler(t)
	created := trip.Trip{
		ID:        uuid.New(),
		Status:    trip.StatusActive,
		Price:     1450,
		StartedAt: time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC),
	}

	uc.EXPECT().Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in model.CreateTripInput) (trip.Trip, error) {
			require.Equal(t, int64(1450), in.Price)
			require.Equal(t, 59.9398, in.Start.Latitude)
			require.Equal(t, uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5"), in.DriverID)
			return created, nil
		})

	rec := post(h, validBody)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "/api/v1/trips/"+created.ID.String(), rec.Header().Get("Location"))

	var body api.Trip
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, api.TripStatus("active"), body.Status)
	require.Equal(t, int64(1450), body.Price)
}

func TestHandle_InvalidBody(t *testing.T) {
	cases := map[string]string{
		"not json":          `not json`,
		"missing price":     strings.Replace(validBody, `,`+"\n"+`  "price": 1450`, ``, 1),
		"null price":        strings.Replace(validBody, `"price": 1450`, `"price": null`, 1),
		"fractional price":  strings.Replace(validBody, `"price": 1450`, `"price": 14.5`, 1),
		"unknown field":     strings.Replace(validBody, `"price": 1450`, `"price": 1450, "id": "x"`, 1),
		"bad uuid":          strings.Replace(validBody, `5cb72c04-7650-45c9-a79b-bcdba0631e0c`, `not-a-uuid`, 1),
		"missing longitude": strings.Replace(validBody, `, "longitude": 30.3626`, ``, 1),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			// Usecase не должен вызываться: ожидания в моке не заданы.
			h, _ := newHandler(t)

			rec := post(h, body)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			require.Equal(t, "invalid_request", decodeProblem(t, rec).Code)
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
		{"driver busy", trip.ErrDriverBusy, http.StatusConflict, "driver_busy"},
		{
			"domain validation", &trip.ValidationError{Field: "price", Reason: "must not be negative"},
			http.StatusBadRequest, "invalid_request",
		},
		{"repository failure", errors.New("pq: password authentication failed"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, uc := newHandler(t)
			uc.EXPECT().Create(gomock.Any(), gomock.Any()).Return(trip.Trip{}, tc.err)

			rec := post(h, validBody)

			require.Equal(t, tc.wantStatus, rec.Code)
			require.Equal(t, tc.wantCode, decodeProblem(t, rec).Code)
			require.NotContains(t, rec.Body.String(), "password", "internal error details must not leak")
		})
	}
}
