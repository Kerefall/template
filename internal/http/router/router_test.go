package router_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/router"
)

func newRouter(t *testing.T) http.Handler {
	t.Helper()

	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }
	okWithID := func(w http.ResponseWriter, _ *http.Request, _ uuid.UUID) { w.WriteHeader(http.StatusNoContent) }

	handler, err := router.New(router.Handlers{
		CreateTrip: ok,
		GetTrip:    okWithID,
		FinishTrip: okWithID,
		Health:     ok,
		Ready:      ok,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)

	return handler
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	var p api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &p))

	return p.Code
}

func TestRoutes(t *testing.T) {
	handler := newRouter(t)
	id := uuid.NewString()

	cases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string
	}{
		{"create", http.MethodPost, "/api/v1/trips", http.StatusNoContent, ""},
		{"get", http.MethodGet, "/api/v1/trips/" + id, http.StatusNoContent, ""},
		{"finish", http.MethodPost, "/api/v1/trips/" + id + "/finish", http.StatusNoContent, ""},
		{"health", http.MethodGet, "/health", http.StatusNoContent, ""},
		{"invalid trip id", http.MethodGet, "/api/v1/trips/abc", http.StatusBadRequest, "invalid_request"},
		{"unknown path", http.MethodGet, "/nope", http.StatusNotFound, "not_found"},
		{"wrong method", http.MethodDelete, "/api/v1/trips", http.StatusMethodNotAllowed, "method_not_allowed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))

			require.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantCode != "" {
				require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
				require.Equal(t, tc.wantCode, problemCode(t, rec))
			}
		})
	}
}
