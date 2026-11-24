DROP INDEX audit_events_application_idx;
DROP INDEX audit_events_job_idx;
DROP INDEX audit_events_company_idx;
ALTER TABLE audit_events DROP COLUMN after_hash, DROP COLUMN before_hash, DROP COLUMN application_id, DROP COLUMN job_id, DROP COLUMN company_id;
DROP TABLE ats_boards;
DROP TABLE company_seeds;
DROP TABLE companies;
