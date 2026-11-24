CREATE TABLE source_browser_origins (
    source text NOT NULL REFERENCES source_policies(source),
    origin text NOT NULL,
    PRIMARY KEY (source, origin)
);

CREATE TABLE browser_tasks (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workflow_id text NOT NULL,
    step_run_id text NOT NULL,
    source text NOT NULL,
    origin text NOT NULL,
    action text NOT NULL,
    policy_snapshot_id bigint NOT NULL REFERENCES effective_step_policies(id),
    policy_version bigint NOT NULL,
    deadline timestamptz NOT NULL,
    callback_id text NOT NULL UNIQUE,
    status text NOT NULL CHECK (status IN
        ('CREATED', 'POLICY_CHECKED', 'QUEUED', 'RUNNING', 'AWAITING_USER',
         'SUCCEEDED', 'FAILED', 'UNKNOWN', 'CANCELLED')),
    session_id text,
    lease_until timestamptz,
    evidence_ref uuid,
    created_at timestamptz NOT NULL DEFAULT now()
);
