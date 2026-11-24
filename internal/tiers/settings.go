package tiers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"karun.tech/jobagent/internal/policy"
)

var stepKeys = map[string]bool{
	"discover_company": true, "validate_board": true, "poll_jobs": true, "normalise_jobs": true,
	"enrich_job": true, "match_job": true, "tier_proposal": true, "prepare_resume": true,
	"prepare_answers": true, "inspect_form": true, "fill_form": true, "submit_application": true,
	"reconcile_application": true, "notify_user": true, "ingest_email": true,
}

func clonePolicy(ctx context.Context, tx *sql.Tx, enabled *bool) (int64, error) {
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(1948737201)"); err != nil {
		return 0, err
	}
	var previous int64
	var active bool
	if err := tx.QueryRowContext(ctx, "SELECT version,global_enabled FROM policy_versions ORDER BY version DESC LIMIT 1").Scan(&previous, &active); err != nil {
		return 0, err
	}
	if previous == math.MaxInt64 {
		return 0, errors.New("policy version exhausted")
	}
	if enabled != nil {
		active = *enabled
	}
	next := previous + 1
	if _, err := tx.ExecContext(ctx, "INSERT INTO policy_versions(version,global_enabled) VALUES($1,$2)", next, active); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO step_defaults(policy_version,step_key,mode)
		SELECT $1,step_key,mode FROM step_defaults WHERE policy_version=$2`, next, previous); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tier_step_modes(policy_version,axis,tier_id,step_key,mode)
		SELECT $1,axis,tier_id,step_key,mode FROM tier_step_modes WHERE policy_version=$2`, next, previous); err != nil {
		return 0, err
	}
	return next, nil
}

// SetGlobal creates a new policy version with the emergency stop setting.
func (s Store) SetGlobal(ctx context.Context, enabled bool) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	version, err := clonePolicy(ctx, tx, &enabled)
	if err != nil {
		return 0, err
	}
	if err := record(ctx, tx, "global_policy_changed", nil); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return version, nil
}

// SetMode creates a new policy version with one updated step rule.
func (s Store) SetMode(ctx context.Context, change ModeChange) (int64, error) {
	change.TierID = strings.ToUpper(strings.TrimSpace(change.TierID))
	if !stepKeys[change.StepKey] || (change.Mode != policy.Live && change.Mode != policy.Audit && change.Mode != policy.Off) {
		return 0, ErrInvalid
	}
	if change.Axis == "" && change.TierID != "" {
		return 0, ErrInvalid
	}
	if change.Axis != "" && ((change.Axis != "company" && change.Axis != "role") || !tierIDPattern.MatchString(change.TierID)) {
		return 0, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	version, err := clonePolicy(ctx, tx, nil)
	if err != nil {
		return 0, err
	}
	if change.Axis == "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO step_defaults(policy_version,step_key,mode) VALUES($1,$2,$3)
			ON CONFLICT(policy_version,step_key) DO UPDATE SET mode=EXCLUDED.mode`, version, change.StepKey, change.Mode)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO tier_step_modes(policy_version,axis,tier_id,step_key,mode)
			VALUES($1,$2,$3,$4,$5) ON CONFLICT(policy_version,axis,tier_id,step_key)
			DO UPDATE SET mode=EXCLUDED.mode`, version, change.Axis, change.TierID, change.StepKey, change.Mode)
	}
	if err != nil {
		return 0, fmt.Errorf("save step mode: %w", err)
	}
	if err := record(ctx, tx, "step_policy_changed", nil); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return version, nil
}

// CurrentGlobal returns the active policy version and global enabled flag.
func (s Store) CurrentGlobal(ctx context.Context) (int64, bool, error) {
	var version int64
	var enabled bool
	err := s.DB.QueryRowContext(ctx, "SELECT version,global_enabled FROM policy_versions ORDER BY version DESC LIMIT 1").Scan(&version, &enabled)
	return version, enabled, err
}

// ListRoleFamilies returns up to 100 role families after an ID.
func (s Store) ListRoleFamilies(ctx context.Context, after int64) ([]RoleFamily, error) {
	if after < 0 {
		return nil, ErrInvalid
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT id,name,criteria,role_tier_id FROM role_families WHERE id>$1 ORDER BY id LIMIT 100", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RoleFamily, 0)
	for rows.Next() {
		var value RoleFamily
		if err := rows.Scan(&value.ID, &value.Name, &value.Criteria, &value.TierID); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// PolicyRule is a default or tier-specific rule in a policy version.
type PolicyRule struct {
	PolicyVersion int64       `json:"policy_version"`
	StepKey       string      `json:"step_key"`
	Axis          string      `json:"axis,omitempty"`
	TierID        string      `json:"tier_id,omitempty"`
	Mode          policy.Mode `json:"mode"`
}

// CurrentModes returns the rules from the latest policy version.
func (s Store) CurrentModes(ctx context.Context) ([]PolicyRule, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT policy_version,step_key,'' AS axis,'' AS tier_id,mode
		FROM step_defaults WHERE policy_version=(SELECT max(version) FROM policy_versions)
		UNION ALL
		SELECT policy_version,step_key,axis,tier_id,mode FROM tier_step_modes
		WHERE policy_version=(SELECT max(version) FROM policy_versions)
		ORDER BY step_key,axis,tier_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PolicyRule, 0)
	for rows.Next() {
		var rule PolicyRule
		if err := rows.Scan(&rule.PolicyVersion, &rule.StepKey, &rule.Axis, &rule.TierID, &rule.Mode); err != nil {
			return nil, err
		}
		result = append(result, rule)
	}
	return result, rows.Err()
}

// UpdateRoleFamily updates its name and criteria without changing its tier.
func (s Store) UpdateRoleFamily(ctx context.Context, family RoleFamily) error {
	family.Name = strings.TrimSpace(family.Name)
	if family.ID <= 0 || family.Name == "" || !validObject(family.Criteria) || family.TierID != "" {
		return ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE role_families SET name=$2,criteria=$3 WHERE id=$1", family.ID, family.Name, family.Criteria)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if err := record(ctx, tx, "role_family_updated", nil); err != nil {
		return err
	}
	return tx.Commit()
}

// TierVersions returns a tier's immutable definition history.
func (s Store) TierVersions(ctx context.Context, axis, id string) ([]Tier, error) {
	id = strings.ToUpper(strings.TrimSpace(id))
	if (axis != "company" && axis != "role") || !tierIDPattern.MatchString(id) {
		return nil, ErrInvalid
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT tier_id,axis,name,enabled,priority,settings,version
		FROM policy_tier_versions WHERE axis=$1 AND tier_id=$2 ORDER BY version`, axis, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Tier, 0)
	for rows.Next() {
		var tier Tier
		if err := rows.Scan(&tier.ID, &tier.Axis, &tier.Name, &tier.Enabled, &tier.Priority, &tier.Settings, &tier.Version); err != nil {
			return nil, err
		}
		result = append(result, tier)
	}
	return result, rows.Err()
}
