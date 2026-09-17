BEGIN;
ALTER TABLE orders ADD COLUMN note text;
COMMIT;
CREATE INDEX CONCURRENTLY idx_orders_note ON orders (note);
