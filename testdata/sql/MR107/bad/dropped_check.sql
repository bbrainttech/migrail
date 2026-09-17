ALTER TABLE orders ADD CONSTRAINT orders_status_not_null CHECK (status IS NOT NULL);
ALTER TABLE orders DROP CONSTRAINT orders_status_not_null;
ALTER TABLE orders ALTER COLUMN status SET NOT NULL;
