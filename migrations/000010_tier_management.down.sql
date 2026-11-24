DROP TRIGGER tier_assignments_no_change ON tier_assignments;
DROP TRIGGER tier_versions_no_change ON policy_tier_versions;
DROP TABLE tier_assignments;
DROP TABLE role_families;
ALTER TABLE companies DROP CONSTRAINT companies_tier_fk;
ALTER TABLE companies DROP COLUMN company_tier_axis, DROP COLUMN company_tier_id;
DROP TABLE policy_tier_versions;
DELETE FROM policy_tiers WHERE id='UNASSIGNED';
ALTER TABLE policy_tiers DROP COLUMN settings, DROP COLUMN priority;
