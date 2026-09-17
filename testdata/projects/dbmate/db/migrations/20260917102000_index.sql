-- migrate:up transaction:false
CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status);

-- migrate:down
DROP INDEX idx_orders_status;
