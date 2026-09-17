-- migrail:ignore-file MR403 reason="run in maintenance window"
ALTER TABLE orders ADD COLUMN note text;
