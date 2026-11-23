CREATE TABLE companies (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    domain text NOT NULL CHECK (length(btrim(domain)) > 0)
);
CREATE UNIQUE INDEX companies_domain_key ON companies (lower(domain));

CREATE TABLE company_seeds (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id bigint NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    url text NOT NULL CHECK (length(btrim(url)) > 0),
    UNIQUE (company_id, url)
);

CREATE TABLE ats_boards (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    company_id bigint NOT NULL REFERENCES companies(id) ON DELETE RESTRICT,
    provider text NOT NULL CHECK (length(btrim(provider)) > 0),
    region text NOT NULL DEFAULT '',
    board_key text NOT NULL CHECK (length(btrim(board_key)) > 0),
    UNIQUE (provider, region, board_key)
);
CREATE INDEX ats_boards_company_idx ON ats_boards(company_id);

ALTER TABLE audit_events
    ADD COLUMN company_id bigint,
    ADD COLUMN job_id bigint,
    ADD COLUMN application_id bigint,
    ADD COLUMN before_hash text,
    ADD COLUMN after_hash text;
CREATE INDEX audit_events_company_idx ON audit_events(company_id, id);
CREATE INDEX audit_events_job_idx ON audit_events(job_id, id);
CREATE INDEX audit_events_application_idx ON audit_events(application_id, id);
