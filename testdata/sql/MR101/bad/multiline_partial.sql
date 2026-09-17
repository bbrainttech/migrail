CREATE INDEX
  idx_users_lower_email
  ON users (lower(email))
  WHERE deleted_at IS NULL;
