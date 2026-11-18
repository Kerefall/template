package trip

import (
	"context"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/Kerefall/template/internal/domain/position"
	domain "github.com/Kerefall/template/internal/domain/trip"
	"github.com/Kerefall/template/internal/model"
)

// lastActivity - время последней активности поездки: последняя координата, а если
// координат не было, то started_at. Выражение должно совпадать с индексом
// trips_active_last_activity_idx, иначе PostgreSQL не сможет им воспользоваться.
const lastActivity = "COALESCE(last_position_at, started_at)"

// TouchLastPosition двигает last_position_at вперёд, но никогда назад: сравнение
// делает сам запрос (GREATEST), поэтому опоздавшая старая координата не
// состарит поездку, а два параллельных запроса не затрут друг друга.
// Завершённая поездка координат не принимает: domain.ErrTripCompleted.
func (r *Repository) TouchLastPosition(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error {
	query, args, err := psql.
		Update("trips").
		Set("last_position_at", squirrel.Expr("GREATEST(last_position_at, ?)", recordedAt)).
		Where(squirrel.Eq{"id": tripID, "status": string(domain.StatusActive)}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update last_position_at: %w", err)
	}

	tag, err := r.db(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update last_position_at: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	// Ни одной строки не обновилось: поездки нет либо она уже завершена.
	if _, err := r.Get(ctx, tripID); err != nil {
		return err
	}

	return domain.ErrTripCompleted
}

// SavePosition вставляет точку маршрута и возвращает её с ID и created_at.
func (r *Repository) SavePosition(ctx context.Context, p position.Position) (position.Position, error) {
	query, args, err := psql.
		Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(p.TripID, p.Point.Latitude, p.Point.Longitude, p.RecordedAt).
		Suffix("RETURNING id, created_at").
		ToSql()
	if err != nil {
		return position.Position{}, fmt.Errorf("build insert position: %w", err)
	}

	if err := r.db(ctx).QueryRow(ctx, query, args...).Scan(&p.ID, &p.CreatedAt); err != nil {
		return position.Position{}, fmt.Errorf("insert position: %w", err)
	}
	p.CreatedAt = p.CreatedAt.UTC()

	return p, nil
}

// ListPositions возвращает все точки маршрута: по recorded_at, затем по id.
func (r *Repository) ListPositions(ctx context.Context, tripID uuid.UUID) ([]position.Position, error) {
	query, args, err := psql.
		Select("id", "trip_id", "latitude", "longitude", "recorded_at", "created_at").
		From("trip_positions").
		Where(squirrel.Eq{"trip_id": tripID}).
		OrderBy("recorded_at", "id").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select positions: %w", err)
	}

	rows, err := r.db(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select positions: %w", err)
	}
	defer rows.Close()

	positions := make([]position.Position, 0)
	for rows.Next() {
		var p position.Position
		if err := rows.Scan(&p.ID, &p.TripID, &p.Point.Latitude, &p.Point.Longitude, &p.RecordedAt, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		p.RecordedAt = p.RecordedAt.UTC()
		p.CreatedAt = p.CreatedAt.UTC()
		positions = append(positions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}

	return positions, nil
}

// ListStale - выборка поездок для опроса одним запросом: активные поездки, у которых
// с момента последней активности прошло больше staleAfter. Время считается в SQL.
// Порядок: сначала те, кто молчит дольше всех. Выборка идёт порциями по limit
// штук, after - позиция, с которой продолжить (nil - с начала).
func (r *Repository) ListStale(ctx context.Context, staleAfter time.Duration, limit int, after *model.StaleCursor) ([]model.StaleTrip, error) {
	// Статус - литерал, а не параметр: иначе PostgreSQL не сможет подобрать
	// частичный индекс (WHERE status = 'active') при обобщённом плане запроса.
	builder := psql.
		Select("id", "driver_id", lastActivity+" AS activity_at").
		From("trips").
		Where("status = 'active'").
		Where(squirrel.Expr(lastActivity+" < now() - make_interval(secs => ?)", staleAfter.Seconds())).
		OrderBy(lastActivity, "id").
		Limit(uint64(limit)) //nolint:gosec // limit проверяется в конфиге: он положительный

	if after != nil {
		builder = builder.Where(squirrel.Expr("("+lastActivity+", id) > (?, ?)", after.ActivityAt, after.ID))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select stale trips: %w", err)
	}

	rows, err := r.db(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select stale trips: %w", err)
	}
	defer rows.Close()

	trips := make([]model.StaleTrip, 0, limit)
	for rows.Next() {
		var t model.StaleTrip
		if err := rows.Scan(&t.ID, &t.DriverID, &t.ActivityAt); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}
		t.ActivityAt = t.ActivityAt.UTC()
		trips = append(trips, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read stale trips: %w", err)
	}

	return trips, nil
}
