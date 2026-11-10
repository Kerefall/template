package trip

import (
	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domain "github.com/Kerefall/template/internal/domain/trip"
)

// Плейсхолдеры $1, $2, ... - так их понимает PostgreSQL.
var psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

var tripColumns = []string{
	"id",
	"user_id",
	"driver_id",
	"start_latitude",
	"start_longitude",
	"end_latitude",
	"end_longitude",
	"price",
	"status",
	"started_at",
	"finished_at",
	"created_at",
	"updated_at",
}

func scanTrip(row pgx.Row) (domain.Trip, error) {
	var (
		t      domain.Trip
		status string
	)

	err := row.Scan(
		&t.ID,
		&t.UserID,
		&t.DriverID,
		&t.Start.Latitude,
		&t.Start.Longitude,
		&t.End.Latitude,
		&t.End.Longitude,
		&t.Price,
		&status,
		&t.StartedAt,
		&t.FinishedAt,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		return domain.Trip{}, err
	}

	t.Status = domain.Status(status)
	t.StartedAt = t.StartedAt.UTC()
	t.CreatedAt = t.CreatedAt.UTC()
	t.UpdatedAt = t.UpdatedAt.UTC()
	if t.FinishedAt != nil {
		finishedAt := t.FinishedAt.UTC()
		t.FinishedAt = &finishedAt
	}

	return t, nil
}
