ALTER TABLE orders ADD CONSTRAINT orders_total_not_null CHECK (total IS NOT NULL);
ALTER TABLE orders ALTER COLUMN status SET NOT NULL;
