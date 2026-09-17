BEGIN;
SET LOCAL lock_timeout TO '2s';
ALTER TABLE orders ADD COLUMN note text;
COMMIT;
