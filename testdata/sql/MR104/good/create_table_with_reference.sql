CREATE TABLE line_items (id bigint PRIMARY KEY, order_id bigint REFERENCES orders (id));
