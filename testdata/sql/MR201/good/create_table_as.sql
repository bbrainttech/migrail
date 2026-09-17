CREATE TABLE order_totals AS SELECT id, total FROM orders;
ALTER TABLE order_totals ALTER COLUMN total TYPE numeric(12, 2);
