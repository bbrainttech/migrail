CREATE TABLE refunds (id bigint);
ALTER TABLE refunds ADD COLUMN token uuid DEFAULT gen_random_uuid();
