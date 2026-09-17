ALTER TABLE orders
  ADD CONSTRAINT orders_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id),
  ADD CONSTRAINT orders_shop_id_fkey FOREIGN KEY (shop_id) REFERENCES shops (id);
