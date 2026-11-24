CREATE TABLE source_policies (
    source text PRIMARY KEY CHECK (length(btrim(source)) > 0),
    discovery_allowed boolean NOT NULL DEFAULT false,
    poll_allowed boolean NOT NULL DEFAULT false,
    api_submit_allowed boolean NOT NULL DEFAULT false,
    browser_submit_allowed boolean NOT NULL DEFAULT false,
    evidence text,
    last_reviewed_at timestamptz
);
