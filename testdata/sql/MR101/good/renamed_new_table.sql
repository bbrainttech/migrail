CREATE TABLE shipments_tmp (id bigint PRIMARY KEY, order_id bigint NOT NULL);
ALTER TABLE shipments_tmp RENAME TO shipments;
CREATE INDEX idx_shipments_order_id ON shipments (order_id);
