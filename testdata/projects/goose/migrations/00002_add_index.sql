-- +goose Up
CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status);

-- +goose Down
DROP INDEX idx_orders_status;
