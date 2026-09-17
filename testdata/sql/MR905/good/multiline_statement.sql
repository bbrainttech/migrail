-- migrail:ignore MR101 reason="empty"
CREATE INDEX idx_orders_status
  ON orders (status);
