package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	api "github.com/Kerefall/template/internal/generated"
	"github.com/Kerefall/template/internal/repository"
	"github.com/Kerefall/template/internal/txmanager"
)

// Pinger проверяет доступность бд. Его реализует *pgxpool.Pool
type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	api.Unimplemented
	DB           Pinger
	Trips        repository.TripRepository
	Tx           txmanager.TxManager
	QueryTimeout time.Duration
	Log          *slog.Logger
}

var _ api.ServerInterface = Handler{}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
