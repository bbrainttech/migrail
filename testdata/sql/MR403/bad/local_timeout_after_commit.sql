BEGIN;
SET LOCAL lock_timeout = '5s';
ALTER TABLE orders ADD COLUMN a text;
COMMIT;
ALTER TABLE users ADD COLUMN b text;
