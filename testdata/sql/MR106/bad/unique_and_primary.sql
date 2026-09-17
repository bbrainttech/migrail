ALTER TABLE orders
  ADD CONSTRAINT orders_code_key UNIQUE (code),
  ADD PRIMARY KEY (id);
