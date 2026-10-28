package domainerror_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Kerefall/template/internal/domain/trip"
	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"trip not found", trip.ErrTripNotFound, http.StatusNotFound, "trip_not_found"},
		{"trip completed", trip.ErrTripCompleted, http.StatusConflict, "trip_completed"},
		{"driver busy", trip.ErrDriverBusy, http.StatusConflict, "driver_busy"},
		{"wrapped domain error", fmt.Errorf("create trip: %w", trip.ErrDriverBusy), http.StatusConflict, "driver_busy"},
		{"validation error", &trip.ValidationError{Field: "price", Reason: "must not be negative"}, http.StatusBadRequest, "invalid_request"},
		{"invalid request", domainerror.InvalidRequest("field \"price\" is required"), http.StatusBadRequest, "invalid_request"},
		{"route not found", domainerror.ErrRouteNotFound, http.StatusNotFound, "not_found"},
		{"method not allowed", domainerror.ErrMethodNotAllowed, http.StatusMethodNotAllowed, "method_not_allowed"},
		{"unknown error", errors.New("boom"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := domainerror.Problem(tc.err)

			require.Equal(t, tc.wantStatus, status)
			require.Equal(t, tc.wantCode, body.Code)
			require.Equal(t, int32(tc.wantStatus), body.Status)
			require.NotEmpty(t, body.Type)
			require.NotEmpty(t, body.Title)
		})
	}
}

func TestProblem_ValidationDetailNamesTheField(t *testing.T) {
	_, body := domainerror.Problem(&trip.ValidationError{Field: "price", Reason: "must not be negative"})

	require.NotNil(t, body.Detail)
	require.Equal(t, "price must not be negative", *body.Detail)
}

func TestWrite_InternalErrorDoesNotLeakDetails(t *testing.T) {
	secret := "pq: password authentication failed for user tripgo, SELECT * FROM trips"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", nil)
	rec := httptest.NewRecorder()

	domainerror.Write(rec, req, nil, errors.New(secret))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.NotContains(t, rec.Body.String(), "password")
	require.NotContains(t, rec.Body.String(), "SELECT")

	var body api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "internal_error", body.Code)
	require.NotNil(t, body.Instance)
	require.Equal(t, "/api/v1/trips", *body.Instance)
}
