ALTER TABLE orders
  ALTER COLUMN user_id TYPE bigint,
  ALTER COLUMN external_id TYPE uuid USING external_id::uuid;
