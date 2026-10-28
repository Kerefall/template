// Package trip - поездка и её бизнес-правила. Ничего не знает про
// HTTP, базу данных и телеметрию.
package trip

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Point struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DriverID   uuid.UUID
	Start      Point
	End        Point
	Price      int64
	Status     Status
	StartedAt  time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// New создаёт активную поездку и проверяет правила модели.
func New(id, userID, driverID uuid.UUID, start, end Point, price int64, now time.Time) (Trip, error) {
	if userID == uuid.Nil {
		return Trip{}, &ValidationError{Field: "user_id", Reason: "must not be empty"}
	}
	if driverID == uuid.Nil {
		return Trip{}, &ValidationError{Field: "driver_id", Reason: "must not be empty"}
	}
	if err := start.validate("start_point"); err != nil {
		return Trip{}, err
	}
	if err := end.validate("end_point"); err != nil {
		return Trip{}, err
	}
	if price < 0 {
		return Trip{}, &ValidationError{Field: "price", Reason: "must not be negative"}
	}

	return Trip{
		ID:        id,
		UserID:    userID,
		DriverID:  driverID,
		Start:     start,
		End:       end,
		Price:     price,
		Status:    StatusActive,
		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (p Point) validate(name string) error {
	if p.Latitude < -90 || p.Latitude > 90 {
		return &ValidationError{Field: name + ".latitude", Reason: "must be between -90 and 90"}
	}
	if p.Longitude < -180 || p.Longitude > 180 {
		return &ValidationError{Field: name + ".longitude", Reason: "must be between -180 and 180"}
	}

	return nil
}

// Finish переводит поездку из active в completed. Это единственный
// разрешённый переход.
func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}

	t.Status = StatusCompleted
	t.FinishedAt = &at
	t.UpdatedAt = at

	return nil
}
