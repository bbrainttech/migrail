CREATE TABLE profiles_tmp (id bigint PRIMARY KEY, bio text);
ALTER TABLE profiles_tmp RENAME TO profiles;
ALTER TABLE profiles RENAME COLUMN bio TO about;
