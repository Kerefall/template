// Package model - структуры, которые передаются между слоями.
package model

import (
	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/trip"
)

// CreateTripInput - данные для создания поездки, уже разобранные транспортом.
type CreateTripInput struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    trip.Point
	End      trip.Point
	Price    int64
}

// StatusChange - запись журнала переходов статуса. From пустой при создании.
type StatusChange struct {
	TripID uuid.UUID
	From   trip.Status
	To     trip.Status
	Reason string
}
