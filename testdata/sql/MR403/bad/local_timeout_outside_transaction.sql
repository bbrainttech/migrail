SET LOCAL lock_timeout = '5s';
ALTER TABLE orders ADD COLUMN note text;
