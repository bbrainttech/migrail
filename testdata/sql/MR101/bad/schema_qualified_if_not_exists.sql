CREATE INDEX IF NOT EXISTS idx_invoices_data ON billing.invoices USING gin (data);
