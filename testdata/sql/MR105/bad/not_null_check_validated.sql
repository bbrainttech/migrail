ALTER TABLE orders ADD CONSTRAINT orders_status_not_null CHECK (status IS NOT NULL);
