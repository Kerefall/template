package push_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Kerefall/template/internal/gateway/push"
)

func TestRequestPosition_SendsCommand(t *testing.T) {
	driver, tripID := uuid.New(), uuid.New()

	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &got))
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)

	client := push.New(srv.URL, time.Second)
	t.Cleanup(client.Close)

	require.NoError(t, client.RequestPosition(context.Background(), driver, tripID))

	require.Equal(t, "REQUEST_POSITION", got["kind"])
	require.Equal(t, driver.String(), got["recipient_id"])
	require.Equal(t, map[string]any{"trip_id": tripID.String()}, got["data"])
}

func TestRequestPosition_ErrorKinds(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"client error", http.StatusBadRequest, push.ErrClientStatus},
		{"rate limited", http.StatusTooManyRequests, push.ErrClientStatus},
		{"server error", http.StatusServiceUnavailable, push.ErrServerStatus},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			t.Cleanup(srv.Close)

			client := push.New(srv.URL, time.Second)
			t.Cleanup(client.Close)

			err := client.RequestPosition(context.Background(), uuid.New(), uuid.New())

			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestRequestPosition_TransportError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // после закрытия порт никто не слушает

	client := push.New(url, time.Second)
	t.Cleanup(client.Close)

	err := client.RequestPosition(context.Background(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, push.ErrTransport)
}

func TestRequestPosition_Timeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	client := push.New(srv.URL, 50*time.Millisecond)
	t.Cleanup(client.Close)

	err := client.RequestPosition(context.Background(), uuid.New(), uuid.New())

	require.ErrorIs(t, err, push.ErrTransport)
}
