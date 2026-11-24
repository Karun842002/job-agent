CREATE TABLE browser_exceptions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    seed_id bigint NOT NULL REFERENCES company_seeds(id) ON DELETE RESTRICT,
    company_id bigint NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    reason text NOT NULL,
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','RESOLVED')),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX browser_exception_pending_key
    ON browser_exceptions(seed_id, reason) WHERE status = 'PENDING';
