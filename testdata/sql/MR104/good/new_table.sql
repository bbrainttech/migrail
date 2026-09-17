CREATE TABLE line_items (id bigint PRIMARY KEY, order_id bigint);
ALTER TABLE line_items ADD CONSTRAINT line_items_order_id_fkey FOREIGN KEY (order_id) REFERENCES orders (id);
