CREATE TABLE shipments (id bigint PRIMARY KEY, status text);
ALTER TABLE shipments ALTER COLUMN status SET NOT NULL;
