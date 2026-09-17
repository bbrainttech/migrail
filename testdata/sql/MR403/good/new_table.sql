CREATE TABLE refunds (id bigint);
ALTER TABLE refunds ADD COLUMN note text;
CREATE INDEX idx_refunds_note ON refunds (note);
