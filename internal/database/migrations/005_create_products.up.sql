CREATE TABLE products (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    price       DOUBLE PRECISION NOT NULL,
    stock       BIGINT NOT NULL DEFAULT 0,
    category_id BIGINT REFERENCES categories (id) ON UPDATE CASCADE ON DELETE RESTRICT,
    seller_id   BIGINT NOT NULL REFERENCES users (id) ON UPDATE CASCADE ON DELETE RESTRICT,
    status      TEXT NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_products_seller_id ON products (seller_id);
CREATE INDEX idx_products_category_id ON products (category_id);
