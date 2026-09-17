ALTER TABLE orders ADD COLUMN meta jsonb DEFAULT jsonb_build_object('v', 1);
