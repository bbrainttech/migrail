BEGIN;
ALTER TABLE orders ADD CONSTRAINT orders_status_not_null CHECK (status IS NOT NULL) NOT VALID;
SET lock_timeout = '5s';
ALTER TABLE orders VALIDATE CONSTRAINT orders_status_not_null;
COMMIT;
