CREATE TABLE categories (
    id        BIGSERIAL PRIMARY KEY,
    name      TEXT NOT NULL,
    parent_id BIGINT REFERENCES categories (id) ON UPDATE CASCADE ON DELETE SET NULL
);

CREATE UNIQUE INDEX idx_categories_name ON categories (name);
