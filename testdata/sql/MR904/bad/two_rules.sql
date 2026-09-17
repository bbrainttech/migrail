-- migrail:ignore MR101,MR403
CREATE INDEX idx_orders_status ON orders (status);
