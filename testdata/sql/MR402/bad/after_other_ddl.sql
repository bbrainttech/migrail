BEGIN;
ALTER TABLE orders ADD COLUMN note text;
CREATE UNIQUE INDEX CONCURRENTLY idx_orders_code ON orders (code);
COMMIT;
