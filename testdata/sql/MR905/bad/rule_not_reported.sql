-- migrail:ignore MR101 reason="small table"
CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status);
