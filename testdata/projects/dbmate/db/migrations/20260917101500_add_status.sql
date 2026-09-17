-- migrate:up
ALTER TABLE orders ADD COLUMN status text;

-- migrate:down
ALTER TABLE orders DROP COLUMN status;
