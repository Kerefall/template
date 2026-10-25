package domain

import "errors"

const (
	StatusActive    = "active"
	StatusCompleted = "completed"
)

var (
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip is already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")
)
