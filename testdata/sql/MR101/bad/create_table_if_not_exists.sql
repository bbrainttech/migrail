CREATE TABLE IF NOT EXISTS users (id bigint PRIMARY KEY, email text);
CREATE INDEX idx_users_email ON users (email);
