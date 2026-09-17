-- migrail:ignore MR101 reason=""
CREATE INDEX idx_orders_status ON orders (status);
