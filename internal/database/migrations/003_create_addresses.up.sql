CREATE TABLE addresses (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users (id) ON UPDATE CASCADE ON DELETE CASCADE,
    street      TEXT,
    city        TEXT,
    state       TEXT,
    country     TEXT,
    postal_code TEXT,
    is_default  BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_addresses_user_id ON addresses (user_id);
