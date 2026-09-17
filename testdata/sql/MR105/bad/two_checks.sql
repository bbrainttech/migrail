ALTER TABLE orders
  ADD CONSTRAINT a CHECK (total > 0),
  ADD CONSTRAINT b CHECK (quantity > 0);
