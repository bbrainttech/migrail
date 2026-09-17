ALTER TABLE orders ADD COLUMN note text;
DO $$ BEGIN RAISE NOTICE 'done'; END $$;
