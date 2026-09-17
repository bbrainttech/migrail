-- +goose NO TRANSACTION
-- +goose Up
CREATE INDEX CONCURRENTLY idx_orders_created ON orders (created_at);
-- +goose Down
DROP INDEX CONCURRENTLY idx_orders_created;
