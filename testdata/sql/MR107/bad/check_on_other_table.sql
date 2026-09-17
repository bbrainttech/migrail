ALTER TABLE invoices ADD CONSTRAINT invoices_status_not_null CHECK (status IS NOT NULL);
ALTER TABLE orders ALTER COLUMN status SET NOT NULL;
