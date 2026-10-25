package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kerefall/template/internal/domain"
	"github.com/Kerefall/template/internal/txmanager"
)

const (
	pgUniqueViolation       = "23505"
	oneActivePerDriverIndex = "trips_one_active_per_driver_idx"
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

var returningTrip = "RETURNING " + strings.Join(tripColumns, ", ")

type Trip struct {
	UserID         uuid.UUID
	DriverID       uuid.UUID
	ID             uuid.UUID
	StartLatitude  float64
	StartLongitude float64
	EndLatitude    float64
	EndLongitude   float64
	Price          int64
	StartedAt      time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Status         string
}

type TripRepository struct {
	Pool *pgxpool.Pool
}

// db отдаёт транзакцию, если метод вызван внутри TxManager.Do, иначе пул
func (r TripRepository) db(ctx context.Context) txmanager.Querier {
	if tx, ok := txmanager.TxFromContext(ctx); ok {
		return tx
	}
	return r.Pool
}

func scanTrip(row pgx.Row) (Trip, error) {
	var t Trip

	err := row.Scan(
		&t.ID,
		&t.UserID,
		&t.DriverID,
		&t.StartLatitude,
		&t.StartLongitude,
		&t.EndLatitude,
		&t.EndLongitude,
		&t.Price,
		&t.Status,
		&t.StartedAt,
		&t.FinishedAt,
		&t.CreatedAt,
		&t.UpdatedAt,
	)
	if err != nil {
		return Trip{}, err
	}

	t.StartedAt = t.StartedAt.UTC()
	t.CreatedAt = t.CreatedAt.UTC()
	t.UpdatedAt = t.UpdatedAt.UTC()
	if t.FinishedAt != nil {
		finishedAt := t.FinishedAt.UTC()
		t.FinishedAt = &finishedAt
	}

	return t, nil
}

// CreateTrip вставляет активную поездку. Если у водителя уже есть активная,
// уникальный индекс даёт ошибку 23505, она превращается в domain.ErrDriverBusy
func (r TripRepository) CreateTrip(ctx context.Context, t Trip) (Trip, error) {
	query, args, err := psql.
		Insert("trips").
		Columns(
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
		).
		Values(
			t.ID,
			t.UserID,
			t.DriverID,
			t.StartLatitude,
			t.StartLongitude,
			t.EndLatitude,
			t.EndLongitude,
			t.Price,
			domain.StatusActive,
			squirrel.Expr("now()"),
		).
		Suffix(returningTrip).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build insert trip: %w", err)
	}

	created, err := scanTrip(r.db(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == pgUniqueViolation &&
			pgErr.ConstraintName == oneActivePerDriverIndex {
			return Trip{}, domain.ErrDriverBusy
		}
		return Trip{}, fmt.Errorf("insert trip: %w", err)
	}

	return created, nil
}

func (r TripRepository) GetTrip(ctx context.Context, id uuid.UUID) (Trip, error) {
	query, args, err := psql.
		Select(tripColumns...).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	trip, err := scanTrip(r.db(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Trip{}, domain.ErrTripNotFound
		}
		return Trip{}, fmt.Errorf("select trip: %w", err)
	}

	return trip, nil
}

// FinishTrip переводит active -> completed одним UPDATE. Условие status = 'active'
// стоит прямо в запросе, поэтому между проверкой и записью нет окна: при двух
// одновременных вызовах второй дождётся первого, увидит completed и не найдёт
// строку для обновления
func (r TripRepository) FinishTrip(ctx context.Context, id uuid.UUID) (Trip, error) {
	query, args, err := psql.
		Update("trips").
		Set("status", domain.StatusCompleted).
		Set("finished_at", squirrel.Expr("now()")).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{"id": id, "status": domain.StatusActive}).
		Suffix(returningTrip).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build update trip: %w", err)
	}

	finished, err := scanTrip(r.db(ctx).QueryRow(ctx, query, args...))
	if err == nil {
		return finished, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, fmt.Errorf("update trip: %w", err)
	}

	// Ни одной строки не обновилось: поездки нет либо она уже завершена.
	// Различаем это дополнительным чтением.
	if _, err := r.GetTrip(ctx, id); err != nil {
		return Trip{}, err
	}
	return Trip{}, domain.ErrTripCompleted
}

// AddStatusHistory пишет строку в журнал переходов статуса
// При создании поездки fromStatus пустой: прежнего статуса нет
func (r TripRepository) AddStatusHistory(ctx context.Context, tripID uuid.UUID, fromStatus, toStatus, reason string) error {
	var from any
	if fromStatus != "" {
		from = fromStatus
	}

	query, args, err := psql.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(tripID, from, toStatus, reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert status history: %w", err)
	}

	if _, err := r.db(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert status history: %w", err)
	}

	return nil
}
