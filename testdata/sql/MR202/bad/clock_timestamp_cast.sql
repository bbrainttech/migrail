ALTER TABLE orders ADD COLUMN seen_at timestamptz DEFAULT clock_timestamp()::timestamptz;
