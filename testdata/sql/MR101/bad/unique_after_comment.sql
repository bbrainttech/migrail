-- speed up lookups by email
CREATE UNIQUE INDEX idx_users_email ON users (email);
