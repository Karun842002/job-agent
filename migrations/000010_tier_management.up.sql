ALTER TABLE policy_tiers
    ADD COLUMN priority integer NOT NULL DEFAULT 0,
    ADD COLUMN settings jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(settings) = 'object');

INSERT INTO policy_tiers (id,axis,name,enabled,version,priority)
VALUES ('UNASSIGNED','company','Unassigned',true,1,0),
       ('UNASSIGNED','role','Unassigned',true,1,0)
ON CONFLICT (axis,id) DO NOTHING;

CREATE TABLE policy_tier_versions (
    axis text NOT NULL,
    tier_id text NOT NULL,
    version bigint NOT NULL,
    name text NOT NULL,
    enabled boolean NOT NULL,
    priority integer NOT NULL,
    settings jsonb NOT NULL,
    actor text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (axis,tier_id,version),
    FOREIGN KEY (axis,tier_id) REFERENCES policy_tiers(axis,id)
);
INSERT INTO policy_tier_versions(axis,tier_id,version,name,enabled,priority,settings,actor)
SELECT axis,id,version,name,enabled,priority,settings,'system' FROM policy_tiers;

ALTER TABLE companies ADD COLUMN company_tier_id text NOT NULL DEFAULT 'UNASSIGNED';
ALTER TABLE companies ADD COLUMN company_tier_axis text GENERATED ALWAYS AS ('company'::text) STORED;
ALTER TABLE companies ADD CONSTRAINT companies_tier_fk FOREIGN KEY (company_tier_axis,company_tier_id) REFERENCES policy_tiers(axis,id);

CREATE TABLE role_families (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    criteria jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(criteria) = 'object'),
    role_tier_id text NOT NULL DEFAULT 'UNASSIGNED',
    role_tier_axis text GENERATED ALWAYS AS ('role'::text) STORED,
    FOREIGN KEY (role_tier_axis,role_tier_id) REFERENCES policy_tiers(axis,id)
);

CREATE TABLE tier_assignments (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    axis text NOT NULL CHECK (axis IN ('company','role')),
    target_id bigint NOT NULL,
    tier_id text NOT NULL,
    tier_version bigint NOT NULL,
    actor text NOT NULL,
    reason text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (axis,tier_id,tier_version) REFERENCES policy_tier_versions(axis,tier_id,version)
);
CREATE INDEX tier_assignments_target_idx ON tier_assignments(axis,target_id,id);

INSERT INTO tier_assignments(axis,target_id,tier_id,tier_version,actor,reason)
SELECT 'company',id,'UNASSIGNED',1,'system','initial_default' FROM companies;

CREATE TRIGGER tier_versions_no_change BEFORE UPDATE OR DELETE ON policy_tier_versions
FOR EACH ROW EXECUTE FUNCTION protect_audit_events();
CREATE TRIGGER tier_assignments_no_change BEFORE UPDATE OR DELETE ON tier_assignments
FOR EACH ROW EXECUTE FUNCTION protect_audit_events();

INSERT INTO policy_versions(version,global_enabled)
SELECT 1,false WHERE NOT EXISTS (SELECT 1 FROM policy_versions);

INSERT INTO step_defaults(policy_version,step_key,mode)
SELECT (SELECT max(version) FROM policy_versions), step_key, 'OFF'
FROM unnest(ARRAY['discover_company','validate_board','poll_jobs','normalise_jobs',
'enrich_job','match_job','tier_proposal','prepare_resume','prepare_answers','inspect_form',
'fill_form','submit_application','reconcile_application','notify_user','ingest_email']) AS step_key
ON CONFLICT DO NOTHING;
