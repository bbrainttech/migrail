ALTER TABLE orders ADD COLUMN qty int NOT NULL CHECK (qty > 0);
