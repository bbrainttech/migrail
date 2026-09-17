ALTER TABLE orders ADD COLUMN created_at timestamptz DEFAULT now();
