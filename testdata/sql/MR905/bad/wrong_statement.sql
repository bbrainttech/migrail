-- migrail:ignore MR101 reason="small table"
SELECT 1;
CREATE INDEX idx_orders_status ON orders (status);
