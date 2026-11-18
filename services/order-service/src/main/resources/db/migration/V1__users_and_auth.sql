CREATE EXTENSION IF NOT EXISTS postgis;

CREATE TABLE users (
    id          uuid PRIMARY KEY,
    phone       varchar(16) NOT NULL,
    role        varchar(16) NOT NULL CHECK (role IN ('CUSTOMER', 'PRO', 'OPS')),
    name        varchar(80),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (phone, role)
);

CREATE TABLE otp_challenges (
    id          uuid PRIMARY KEY,
    phone       varchar(16) NOT NULL,
    role        varchar(16) NOT NULL,
    code_hash   varchar(64) NOT NULL,
    attempts    int NOT NULL DEFAULT 0,
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX otp_challenges_phone_created ON otp_challenges (phone, created_at DESC);

CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    family_id   uuid NOT NULL,
    token_hash  varchar(64) NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    replaced_by uuid,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_family ON refresh_tokens (family_id);

CREATE TABLE consents (
    id          uuid PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose     varchar(48) NOT NULL,
    granted     boolean NOT NULL,
    notice_version varchar(16) NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX consents_user_purpose ON consents (user_id, purpose, created_at DESC);
