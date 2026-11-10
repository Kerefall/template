package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	domain "github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/model"
)

const (
	pgUniqueViolation       = "23505"
	oneActivePerDriverIndex = "trips_one_active_per_driver_idx"
)

// Save вставляет новую поездку. Если у водителя уже есть активная, уникальный
// индекс даёт ошибку 23505, она превращается в domain.ErrDriverBusy.
func (r *Repository) Save(ctx context.Context, t domain.Trip) error {
	query, args, err := psql.
		Insert("trips").
		Columns(tripColumns...).
		Values(
			t.ID,
			t.UserID,
			t.DriverID,
			t.Start.Latitude,
			t.Start.Longitude,
			t.End.Latitude,
			t.End.Longitude,
			t.Price,
			string(t.Status),
			t.StartedAt,
			t.FinishedAt,
			t.CreatedAt,
			t.UpdatedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip: %w", err)
	}

	if _, err := r.db(ctx).Exec(ctx, query, args...); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) &&
			pgErr.Code == pgUniqueViolation &&
			pgErr.ConstraintName == oneActivePerDriverIndex {
			return domain.ErrDriverBusy
		}

		return fmt.Errorf("insert trip: %w", err)
	}

	return nil
}

// Update сохраняет изменяемые поля поездки: статус, время завершения и обновления.
func (r *Repository) Update(ctx context.Context, t domain.Trip) error {
	query, args, err := psql.
		Update("trips").
		Set("status", string(t.Status)).
		Set("finished_at", t.FinishedAt).
		Set("updated_at", t.UpdatedAt).
		Where(squirrel.Eq{"id": t.ID}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update trip: %w", err)
	}

	tag, err := r.db(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update trip: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTripNotFound
	}

	return nil
}

// AddStatusHistory пишет строку в журнал переходов статуса.
func (r *Repository) AddStatusHistory(ctx context.Context, change model.StatusChange) error {
	var from any
	if change.From != "" {
		from = string(change.From)
	}

	query, args, err := psql.
		Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason").
		Values(change.TripID, from, string(change.To), change.Reason).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert status history: %w", err)
	}

	if _, err := r.db(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert status history: %w", err)
	}

	return nil
}
