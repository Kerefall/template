// Package trip - хранение поездок в PostgreSQL.
package trip

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Kerefall/template/internal/infra/postgres/transaction"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// db отдаёт транзакцию, если метод вызван внутри transaction.Manager.Do,
// иначе пул.
func (r *Repository) db(ctx context.Context) transaction.Querier {
	return transaction.QuerierFromContext(ctx, r.pool)
}
