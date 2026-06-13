CREATE TABLE order_items (
    id         BIGSERIAL PRIMARY KEY,
    order_id   BIGINT NOT NULL REFERENCES orders (id) ON UPDATE CASCADE ON DELETE CASCADE,
    product_id BIGINT NOT NULL REFERENCES products (id) ON UPDATE CASCADE ON DELETE RESTRICT,
    quantity   BIGINT,
    price      DOUBLE PRECISION
);

CREATE INDEX idx_order_items_order_id ON order_items (order_id);
CREATE INDEX idx_order_items_product_id ON order_items (product_id);
