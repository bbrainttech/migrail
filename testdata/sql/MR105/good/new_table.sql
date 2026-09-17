CREATE TABLE refunds (id bigint, amount numeric);
ALTER TABLE refunds ADD CONSTRAINT refunds_amount_positive CHECK (amount > 0);
