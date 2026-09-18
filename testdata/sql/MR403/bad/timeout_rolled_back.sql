BEGIN;
SET lock_timeout = '5s';
ROLLBACK;
ALTER TABLE orders ADD COLUMN note text;
