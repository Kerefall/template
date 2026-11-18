package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/trip"
)

// SavePositionInput - данные для сохранения координаты, разобранные транспортом.
type SavePositionInput struct {
	TripID     uuid.UUID
	Point      trip.Point
	RecordedAt time.Time
}

// StaleTrip - активная поездка, по которой давно не было координат.
// ActivityAt - время последней координаты, а если координат не было - started_at.
type StaleTrip struct {
	ID         uuid.UUID
	DriverID   uuid.UUID
	ActivityAt time.Time
}

// StaleCursor - позиция, с которой продолжается выборка устаревших поездок
// (по паре ActivityAt, ID, поэтому одинаковое время на границе порций не теряется).
type StaleCursor struct {
	ActivityAt time.Time
	ID         uuid.UUID
}
