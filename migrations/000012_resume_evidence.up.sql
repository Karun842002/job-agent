-- Initial structured facts used by deterministic relevance and later resume
-- curation. There is deliberately no candidate-data ingestion endpoint here.
CREATE TABLE career_evidence (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    fact_key text NOT NULL CHECK (length(btrim(fact_key)) > 0),
    kind text NOT NULL CHECK (kind IN ('experience', 'preferred_location', 'target_title', 'skill')),
    value text NOT NULL CHECK (length(btrim(value)) > 0),
    source_ref text NOT NULL CHECK (length(btrim(source_ref)) > 0),
    version bigint NOT NULL CHECK (version > 0),
    verified boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (fact_key, version)
);

CREATE FUNCTION reject_career_evidence_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'career evidence versions cannot be updated';
END;
$$;
CREATE TRIGGER career_evidence_no_update BEFORE UPDATE ON career_evidence
FOR EACH ROW EXECUTE FUNCTION reject_career_evidence_update();

-- Source reliability is an explicit assessment, separate from permission to
-- fetch it. A missing assessment is unknown and cannot qualify a proposal.
CREATE TABLE source_reliability (
    source text PRIMARY KEY REFERENCES source_policies(source) ON DELETE RESTRICT,
    reliable boolean NOT NULL,
    evidence_ref text NOT NULL CHECK (length(btrim(evidence_ref)) > 0),
    reviewed_at timestamptz NOT NULL
);
