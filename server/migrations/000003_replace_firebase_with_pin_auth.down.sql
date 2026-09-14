DROP TABLE IF EXISTS refresh_sessions;
TRUNCATE TABLE job_payments, jobs, users CASCADE;
ALTER TABLE users
    DROP COLUMN IF EXISTS locked_until,
    DROP COLUMN IF EXISTS failed_pin_attempts,
    DROP COLUMN IF EXISTS pin_hash,
    ADD COLUMN firebase_uid TEXT NOT NULL UNIQUE;
