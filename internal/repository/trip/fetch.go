package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domain "github.com/Kerefall/template/internal/domain/trip"
)

// Get возвращает поездку или domain.ErrTripNotFound.
func (r *Repository) Get(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	return r.fetch(ctx, id, false)
}

// GetForUpdate читает поездку и блокирует строку до конца транзакции.
// Так два одновременных завершения идут по очереди: второе увидит
// уже завершённую поездку.
func (r *Repository) GetForUpdate(ctx context.Context, id uuid.UUID) (domain.Trip, error) {
	return r.fetch(ctx, id, true)
}

func (r *Repository) fetch(ctx context.Context, id uuid.UUID, lock bool) (domain.Trip, error) {
	builder := psql.
		Select(tripColumns...).
		From("trips").
		Where(squirrel.Eq{"id": id})
	if lock {
		builder = builder.Suffix("FOR UPDATE")
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	t, err := scanTrip(r.db(ctx).QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Trip{}, domain.ErrTripNotFound
		}

		return domain.Trip{}, fmt.Errorf("select trip: %w", err)
	}

	return t, nil
}
