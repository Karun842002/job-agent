CREATE TABLE audit_previews (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    policy_snapshot_id bigint NOT NULL REFERENCES effective_step_policies(id),
    step_run_id text NOT NULL,
    diff jsonb NOT NULL
);
