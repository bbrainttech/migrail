ALTER TABLE orders
  ADD COLUMN seq bigint DEFAULT nextval('orders_seq');
