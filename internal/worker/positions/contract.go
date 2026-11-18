//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}
package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/model"
)

type (
	// staleRepository отдаёт устаревшие поездки порциями, начиная с тех,
	// что молчат дольше всех.
	staleRepository interface {
		ListStale(ctx context.Context, staleAfter time.Duration, limit int, after *model.StaleCursor) ([]model.StaleTrip, error)
	}

	// pushGateway просит устройство водителя прислать координату.
	pushGateway interface {
		RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error
	}

	metrics interface {
		StaleCheck(ctx context.Context, result string)
		RequestFinished(ctx context.Context, success bool, d time.Duration)
		RequestDropped(ctx context.Context)
	}

	// Ticker - тикер за интерфейсом: в тестах подставляется управляемая реализация.
	Ticker interface {
		C() <-chan time.Time
		Stop()
	}
)
