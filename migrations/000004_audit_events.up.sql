CREATE TABLE audit_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at timestamptz NOT NULL DEFAULT now(),
    event_name text NOT NULL,
    actor text NOT NULL,
    cause text NOT NULL,
    policy_snapshot_id bigint REFERENCES effective_step_policies(id),
    trace_id text,
    step_run_id text,
    evidence_ref uuid
);

CREATE FUNCTION protect_audit_events() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' AND OLD.created_at <= now() - interval '24 hours' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'audit events are append-only during their retention period';
END;
$$;

CREATE TRIGGER audit_events_no_change
BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION protect_audit_events();

CREATE INDEX audit_events_step_run_idx ON audit_events(step_run_id, id);
