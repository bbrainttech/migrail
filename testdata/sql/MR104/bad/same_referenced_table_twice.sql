ALTER TABLE posts ADD FOREIGN KEY (author_id) REFERENCES users (id), ADD FOREIGN KEY (editor_id) REFERENCES users (id);
