ALTER TABLE ats_boards
    ADD COLUMN state text NOT NULL DEFAULT 'candidate'
        CHECK (state IN ('candidate','verified','degraded','inactive','blocked_by_policy','manual_review')),
    ADD COLUMN capabilities jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(capabilities) = 'object'),
    ADD COLUMN provenance jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(provenance) = 'object'),
    ADD COLUMN last_verified_at timestamptz,
    ADD COLUMN operator_override jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(operator_override) = 'object');
