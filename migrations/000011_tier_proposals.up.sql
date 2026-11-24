CREATE TABLE tier_proposals (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id bigint NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    role_family_id bigint REFERENCES role_families(id) ON DELETE RESTRICT,
    evidence jsonb NOT NULL CHECK (jsonb_typeof(evidence) = 'object'),
    source_url text NOT NULL CHECK (length(btrim(source_url)) > 0),
    suggested_tier_id text,
    explanation text NOT NULL CHECK (length(btrim(explanation)) > 0),
    confidence numeric CHECK (confidence >= 0 AND confidence <= 1),
    status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ASSIGNED','EXCLUDED','SNOOZED')),
    notified_at timestamptz,
    decision_actor text,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL CHECK (expires_at > created_at)
);

-- NULL is a company proposal; a role family is proposed once across sources.
CREATE UNIQUE INDEX tier_proposals_company_key ON tier_proposals(company_id)
    WHERE role_family_id IS NULL;
CREATE UNIQUE INDEX tier_proposals_role_key ON tier_proposals(role_family_id)
    WHERE role_family_id IS NOT NULL;
