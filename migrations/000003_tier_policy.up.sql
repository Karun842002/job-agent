CREATE TABLE policy_tiers (
    id text NOT NULL,
    axis text NOT NULL CHECK (axis IN ('company', 'role')),
    name text NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    version bigint NOT NULL CHECK (version > 0),
    PRIMARY KEY (axis, id)
);

CREATE TABLE policy_versions (
    version bigint PRIMARY KEY CHECK (version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    global_enabled boolean NOT NULL DEFAULT false
);

CREATE TABLE step_defaults (
    policy_version bigint NOT NULL REFERENCES policy_versions(version),
    step_key text NOT NULL,
    mode text NOT NULL CHECK (mode IN ('LIVE', 'AUDIT', 'OFF')),
    PRIMARY KEY (policy_version, step_key)
);

CREATE TABLE tier_step_modes (
    policy_version bigint NOT NULL REFERENCES policy_versions(version),
    axis text NOT NULL,
    tier_id text NOT NULL,
    step_key text NOT NULL,
    mode text NOT NULL CHECK (mode IN ('LIVE', 'AUDIT', 'OFF')),
    PRIMARY KEY (policy_version, axis, tier_id, step_key),
    FOREIGN KEY (axis, tier_id) REFERENCES policy_tiers(axis, id)
);

CREATE TABLE effective_step_policies (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    step_key text NOT NULL,
    policy_version bigint NOT NULL REFERENCES policy_versions(version),
    inputs jsonb NOT NULL,
    mode text NOT NULL CHECK (mode IN ('LIVE', 'AUDIT', 'OFF')),
    reason text NOT NULL
);

CREATE FUNCTION reject_policy_snapshot_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'effective policy snapshots cannot be updated';
END;
$$;

CREATE TRIGGER effective_policy_no_update
BEFORE UPDATE ON effective_step_policies
FOR EACH ROW EXECUTE FUNCTION reject_policy_snapshot_update();
