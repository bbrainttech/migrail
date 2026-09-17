-- migrail:ignore-file MR403 reason="maintenance window"
ALTER TABLE orders ADD COLUMN note text;
