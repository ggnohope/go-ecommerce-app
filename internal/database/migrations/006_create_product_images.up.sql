CREATE TABLE product_images (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products (id) ON UPDATE CASCADE ON DELETE CASCADE,
    url        TEXT NOT NULL,
    position   BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX idx_product_images_product_id ON product_images (product_id);
