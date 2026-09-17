SET lock_timeout = '5s';
-- migrail:ignore MR101 reason="orders is empty"
CREATE INDEX idx_orders_status ON orders (status);
