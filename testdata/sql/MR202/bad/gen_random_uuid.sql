ALTER TABLE orders ADD COLUMN token uuid DEFAULT gen_random_uuid();
