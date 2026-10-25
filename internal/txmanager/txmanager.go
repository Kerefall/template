package txmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// rollbackTimeout - сколько ждём ответа бд на ROLLBACK
const rollbackTimeout = 5 * time.Second

// Querier - то, через что репозиторий ходит в бд: пул или транзакция
// И *pgxpool.Pool, и pgx.Tx реализуют этот интерфейс
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TxManager выполняет fn внутри транзакции. Транзакция лежит в контексте,
// который получает fn, поэтому бизнес-код про неё ничего не знает
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type txManager struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) TxManager {
	return &txManager{pool: pool}
}

type txContextKey struct{}

// TxFromContext возвращает транзакцию, если контекст получен из Do
func TxFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(pgx.Tx)
	return tx, ok
}

func (tm *txManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	// Вложенный вызов: транзакция уже открыта выше, работаем в ней же
	// Коммит и откат делает внешний Do
	if _, ok := TxFromContext(ctx); ok {
		return fn(ctx)
	}

	// Уровень изоляции задаём явно: READ COMMITTED. Почему его хватает,
	// описано в README (раздел «Решения»)
	tx, err := tm.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			rollback(ctx, tx)
			panic(p)
		}
	}()

	if err := fn(context.WithValue(ctx, txContextKey{}, tx)); err != nil {
		rollback(ctx, tx)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// rollback откатывает транзакцию. Контекст запроса к этому моменту может быть
// уже отменён (таймаут, обрыв клиента), поэтому берём его без отмены,
// но со своим таймаутом. Ошибку отката не возвращаем: важнее исходная
func rollback(ctx context.Context, tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rollbackTimeout)
	defer cancel()
	_ = tx.Rollback(ctx)
}
