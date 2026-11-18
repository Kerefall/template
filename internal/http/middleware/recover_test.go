package middleware_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/http/middleware"
)

func TestRecover_PanicBecomes500(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := middleware.Recover(log)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret internal detail")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/x", nil)

	require.NotPanics(t, func() { handler.ServeHTTP(rec, req) })

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
	require.NotContains(t, rec.Body.String(), "secret")

	var body api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "internal_error", body.Code)
}

func TestRecover_PassesNormalRequestsThrough(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := middleware.Recover(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	require.Equal(t, http.StatusNoContent, rec.Code)
}
