// Package position - точка маршрута поездки и её правила.
package position

import (
	"time"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/trip"
)

// maxFutureSkew - насколько recorded_at может опережать время сервера
// (часы устройства могут немного расходиться с нашими).
const maxFutureSkew = time.Minute

type Position struct {
	ID         int64
	TripID     uuid.UUID
	Point      trip.Point
	RecordedAt time.Time
	CreatedAt  time.Time
}

// New проверяет правила и создаёт точку маршрута. ID и CreatedAt назначает хранилище.
func New(tripID uuid.UUID, point trip.Point, recordedAt, now time.Time) (Position, error) {
	if err := point.Validate("coordinates"); err != nil {
		return Position{}, err
	}
	if recordedAt.After(now.Add(maxFutureSkew)) {
		return Position{}, &trip.ValidationError{
			Field:  "recorded_at",
			Reason: "must not be more than a minute in the future",
		}
	}

	return Position{
		TripID:     tripID,
		Point:      point,
		RecordedAt: recordedAt.UTC(),
	}, nil
}
