CREATE TABLE orders (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL REFERENCES users (id) ON UPDATE CASCADE ON DELETE RESTRICT,
    status            TEXT NOT NULL DEFAULT 'pending',
    total_amount      DOUBLE PRECISION,
    payment_intent_id TEXT,
    payment_status    TEXT NOT NULL DEFAULT 'pending',
    shipping_address  TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orders_user_id ON orders (user_id);
