CREATE TABLE profiles (id bigint PRIMARY KEY, bio text);
ALTER TABLE profiles RENAME COLUMN bio TO about;
