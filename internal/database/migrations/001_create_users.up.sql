CREATE TABLE users (
    id         BIGSERIAL PRIMARY KEY,
    first_name TEXT,
    last_name  TEXT,
    email      TEXT NOT NULL,
    phone      TEXT,
    password   TEXT,
    code       TEXT,
    expiry     TIMESTAMPTZ,
    verified   BOOLEAN NOT NULL DEFAULT FALSE,
    user_type  TEXT NOT NULL DEFAULT 'buyer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_users_email ON users (email);
