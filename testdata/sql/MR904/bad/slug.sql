-- migrail:ignore create-index-non-concurrent
CREATE INDEX idx_orders_status ON orders (status);
