-- store times with a time zone
ALTER TABLE orders ALTER COLUMN created_at TYPE timestamptz;
