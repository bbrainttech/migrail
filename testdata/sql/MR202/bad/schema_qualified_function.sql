ALTER TABLE orders ADD COLUMN token uuid DEFAULT public.uuid_generate_v4();
