DO $$
BEGIN
  EXECUTE format('ALTER TABLE %I ADD COLUMN x int', 'orders');
END
$$;
