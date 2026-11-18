-- +goose NO TRANSACTION

-- +goose Up
-- Частичный индекс только по активным поездкам, в том же порядке, что и в запросе
-- выборки: время последней активности (последняя координата или started_at), затем id.
CREATE INDEX CONCURRENTLY IF NOT EXISTS trips_active_last_activity_idx
    ON trips ((COALESCE(last_position_at, started_at)), id)
    WHERE status = 'active';

-- +goose Down
DROP INDEX CONCURRENTLY IF EXISTS trips_active_last_activity_idx;
