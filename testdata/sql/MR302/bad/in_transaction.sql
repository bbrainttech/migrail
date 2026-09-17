BEGIN;
ALTER TABLE users ADD COLUMN nickname text;
  ALTER TABLE users RENAME COLUMN users TO username;
COMMIT;
