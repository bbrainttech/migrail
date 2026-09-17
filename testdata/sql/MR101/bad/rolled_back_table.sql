BEGIN;
CREATE TABLE shipments (id bigint PRIMARY KEY, order_id bigint);
ROLLBACK;
CREATE INDEX idx_shipments_order_id ON shipments (order_id);
