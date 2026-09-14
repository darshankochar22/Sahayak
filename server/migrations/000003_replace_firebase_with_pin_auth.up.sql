TRUNCATE TABLE job_payments, jobs, users CASCADE;

ALTER TABLE users
    DROP COLUMN firebase_uid,
    ADD COLUMN pin_hash TEXT NOT NULL,
    ADD COLUMN failed_pin_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_pin_attempts >= 0),
    ADD COLUMN locked_until TIMESTAMPTZ;

CREATE TABLE refresh_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX refresh_sessions_user_id_idx ON refresh_sessions (user_id);
CREATE INDEX refresh_sessions_expires_at_idx ON refresh_sessions (expires_at);
