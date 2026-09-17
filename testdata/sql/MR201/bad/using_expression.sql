ALTER TABLE orders ALTER COLUMN total TYPE numeric(12, 2) USING total::numeric(12, 2);
