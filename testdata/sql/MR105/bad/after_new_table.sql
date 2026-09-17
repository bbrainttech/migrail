CREATE TABLE refunds (id bigint);
ALTER TABLE orders ADD CONSTRAINT orders_total_positive CHECK (total > 0);
