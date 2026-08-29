CREATE TABLE jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hirer_id UUID NOT NULL REFERENCES users(id),
    material_description TEXT NOT NULL,
    required_labourers INTEGER NOT NULL CHECK (required_labourers > 0),
    total_amount_paise BIGINT NOT NULL CHECK (total_amount_paise > 0),
    amount_per_labourer_paise BIGINT NOT NULL CHECK (amount_per_labourer_paise > 0),
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (
        status IN ('DRAFT', 'SEARCHING', 'FILLED', 'IN_PROGRESS', 'COMPLETED', 'CANCELLED', 'EXPIRED')
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (total_amount_paise = amount_per_labourer_paise * required_labourers),
    UNIQUE (id, total_amount_paise)
);

CREATE INDEX jobs_hirer_id_idx ON jobs (hirer_id);
CREATE INDEX jobs_status_idx ON jobs (status);

CREATE TABLE job_payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL UNIQUE,
    amount_paise BIGINT NOT NULL CHECK (amount_paise > 0),
    currency CHAR(3) NOT NULL DEFAULT 'INR' CHECK (currency = 'INR'),
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PAID_OFFLINE')),
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (status = 'PENDING' AND paid_at IS NULL)
        OR (status = 'PAID_OFFLINE' AND paid_at IS NOT NULL)
    ),
    FOREIGN KEY (job_id, amount_paise)
        REFERENCES jobs(id, total_amount_paise)
        ON DELETE CASCADE
);

CREATE INDEX job_payments_status_idx ON job_payments (status);

CREATE FUNCTION create_pending_job_payment()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO job_payments (job_id, amount_paise)
    VALUES (NEW.id, NEW.total_amount_paise);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER jobs_create_pending_payment
AFTER INSERT ON jobs
FOR EACH ROW
EXECUTE FUNCTION create_pending_job_payment();
