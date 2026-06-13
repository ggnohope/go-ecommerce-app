CREATE TABLE cart_items (
    id         BIGSERIAL PRIMARY KEY,
    cart_id    BIGINT NOT NULL REFERENCES carts (id) ON UPDATE CASCADE ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products (id) ON UPDATE CASCADE ON DELETE CASCADE,
    quantity   BIGINT NOT NULL DEFAULT 1,
    price      DOUBLE PRECISION
);

CREATE INDEX idx_cart_items_cart_id ON cart_items (cart_id);
CREATE INDEX idx_cart_items_product_id ON cart_items (product_id);
