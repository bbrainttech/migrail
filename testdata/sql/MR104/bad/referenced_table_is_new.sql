CREATE TABLE shops (id bigint PRIMARY KEY);
ALTER TABLE orders ADD CONSTRAINT orders_shop_id_fkey FOREIGN KEY (shop_id) REFERENCES shops (id);
