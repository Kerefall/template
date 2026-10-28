package ready_test

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Kerefall/template/internal/http/ready"
)

func TestHandle(t *testing.T) {
	cases := []struct {
		name       string
		pingErr    error
		wantStatus int
	}{
		{"database is up", nil, http.StatusOK},
		{"database is down", errors.New("connection refused"), http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := NewMockpinger(gomock.NewController(t))
			db.EXPECT().Ping(gomock.Any()).Return(tc.pingErr)
			h := ready.New(db, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))

			rec := httptest.NewRecorder()
			h.Handle(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

			require.Equal(t, tc.wantStatus, rec.Code)
		})
	}
}
