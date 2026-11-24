// Package tiers manages tier definitions and assignment history.
package tiers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"karun.tech/jobagent/internal/audit"
	"karun.tech/jobagent/internal/policy"
)

var ErrInvalid = errors.New("invalid tier request")
var ErrConflict = errors.New("tier version conflict")
var tierIDPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_-]{0,63}$`)

// Tier is an editable company or role tier. Version is an optimistic lock.
type Tier struct {
	ID       string          `json:"id"`
	Axis     string          `json:"axis"`
	Name     string          `json:"name"`
	Enabled  bool            `json:"enabled"`
	Priority int             `json:"priority"`
	Settings json.RawMessage `json:"settings"`
	Version  int64           `json:"version"`
}

// RoleFamily identifies a configurable role category.
type RoleFamily struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Criteria json.RawMessage `json:"criteria"`
	TierID   string          `json:"tier_id"`
}

// Store persists tier settings and history.
type Store struct{ DB *sql.DB }

func validObject(value json.RawMessage) bool {
	if len(value) == 0 || len(value) > 65536 {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(value, &object) == nil && object != nil
}

func record(ctx context.Context, tx *sql.Tx, name string, companyID *int64) error {
	_, err := audit.Append(ctx, tx, audit.Event{Name: name, Actor: "operator", Cause: "tier_change", CompanyID: companyID})
	return err
}

// SaveTier creates or updates a tier and appends a version record.
func (s Store) SaveTier(ctx context.Context, tier Tier) (Tier, error) {
	tier.ID = strings.ToUpper(strings.TrimSpace(tier.ID))
	tier.Name = strings.TrimSpace(tier.Name)
	if (tier.Axis != "company" && tier.Axis != "role") || !tierIDPattern.MatchString(tier.ID) || tier.Name == "" || !validObject(tier.Settings) || tier.Version < 0 {
		return Tier{}, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Tier{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(1948737201)"); err != nil {
		return Tier{}, err
	}
	var current int64
	err = tx.QueryRowContext(ctx, "SELECT version FROM policy_tiers WHERE axis=$1 AND id=$2 FOR UPDATE", tier.Axis, tier.ID).Scan(&current)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if tier.Version != 0 {
			return Tier{}, ErrConflict
		}
		tier.Version = 1
		_, err = tx.ExecContext(ctx, `INSERT INTO policy_tiers(axis,id,name,enabled,priority,settings,version)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`, tier.Axis, tier.ID, tier.Name, tier.Enabled, tier.Priority, tier.Settings, tier.Version)
	case err != nil:
		return Tier{}, err
	default:
		if current != tier.Version {
			return Tier{}, ErrConflict
		}
		tier.Version = current + 1
		_, err = tx.ExecContext(ctx, `UPDATE policy_tiers SET name=$3,enabled=$4,priority=$5,settings=$6,version=$7
			WHERE axis=$1 AND id=$2`, tier.Axis, tier.ID, tier.Name, tier.Enabled, tier.Priority, tier.Settings, tier.Version)
	}
	if err != nil {
		return Tier{}, fmt.Errorf("save tier: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO policy_tier_versions(axis,tier_id,version,name,enabled,priority,settings,actor)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'operator')`, tier.Axis, tier.ID, tier.Version, tier.Name, tier.Enabled, tier.Priority, tier.Settings)
	if err != nil {
		return Tier{}, err
	}
	if err := record(ctx, tx, "tier_changed", nil); err != nil {
		return Tier{}, err
	}
	if err := tx.Commit(); err != nil {
		return Tier{}, err
	}
	return tier, nil
}

// ListTiers returns all definitions for an axis.
func (s Store) ListTiers(ctx context.Context, axis string) ([]Tier, error) {
	if axis != "company" && axis != "role" {
		return nil, ErrInvalid
	}
	rows, err := s.DB.QueryContext(ctx, "SELECT id,axis,name,enabled,priority,settings,version FROM policy_tiers WHERE axis=$1 ORDER BY priority,id", axis)
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

// CreateRoleFamily creates a role category assigned to UNASSIGNED.
func (s Store) CreateRoleFamily(ctx context.Context, family RoleFamily) (RoleFamily, error) {
	family.Name = strings.TrimSpace(family.Name)
	if family.Name == "" || !validObject(family.Criteria) {
		return RoleFamily{}, ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return RoleFamily{}, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, "INSERT INTO role_families(name,criteria) VALUES($1,$2) RETURNING id,role_tier_id", family.Name, family.Criteria).Scan(&family.ID, &family.TierID); err != nil {
		return RoleFamily{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tier_assignments(axis,target_id,tier_id,tier_version,actor,reason)
		SELECT 'role',$1,id,version,'system','initial_default' FROM policy_tiers WHERE axis='role' AND id='UNASSIGNED'`, family.ID); err != nil {
		return RoleFamily{}, err
	}
	if err := record(ctx, tx, "role_family_created", nil); err != nil {
		return RoleFamily{}, err
	}
	if err := tx.Commit(); err != nil {
		return RoleFamily{}, err
	}
	return family, nil
}

// Assign records a tier assignment without overwriting its history.
func (s Store) Assign(ctx context.Context, axis string, targetID int64, tierID, reason string) error {
	tierID = strings.ToUpper(strings.TrimSpace(tierID))
	reason = strings.TrimSpace(reason)
	if (axis != "company" && axis != "role") || targetID <= 0 || !tierIDPattern.MatchString(tierID) || reason == "" || len(reason) > 512 {
		return ErrInvalid
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(1948737201)"); err != nil {
		return err
	}
	var version int64
	if err := tx.QueryRowContext(ctx, "SELECT version FROM policy_tiers WHERE axis=$1 AND id=$2 FOR SHARE", axis, tierID).Scan(&version); err != nil {
		return err
	}
	var query string
	if axis == "company" {
		query = "UPDATE companies SET company_tier_id=$2 WHERE id=$1"
	} else {
		query = "UPDATE role_families SET role_tier_id=$2 WHERE id=$1"
	}
	result, err := tx.ExecContext(ctx, query, targetID, tierID)
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
	_, err = tx.ExecContext(ctx, `INSERT INTO tier_assignments(axis,target_id,tier_id,tier_version,actor,reason)
		VALUES($1,$2,$3,$4,'operator',$5)`, axis, targetID, tierID, version, reason)
	if err != nil {
		return err
	}
	var companyID *int64
	if axis == "company" {
		companyID = &targetID
	}
	if err := record(ctx, tx, "tier_assigned", companyID); err != nil {
		return err
	}
	return tx.Commit()
}

// Assignment is a historical tier assignment.
type Assignment struct {
	ID          int64  `json:"id"`
	Axis        string `json:"axis"`
	TargetID    int64  `json:"target_id"`
	TierID      string `json:"tier_id"`
	TierVersion int64  `json:"tier_version"`
	Actor       string `json:"actor"`
	Reason      string `json:"reason"`
	CreatedAt   string `json:"created_at"`
}

// Assignments returns up to 100 historical assignments after an ID.
func (s Store) Assignments(ctx context.Context, axis string, targetID, after int64) ([]Assignment, error) {
	if (axis != "company" && axis != "role") || targetID <= 0 || after < 0 {
		return nil, ErrInvalid
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,axis,target_id,tier_id,tier_version,actor,reason,created_at::text
		FROM tier_assignments WHERE axis=$1 AND target_id=$2 AND id>$3 ORDER BY id LIMIT 100`, axis, targetID, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Assignment, 0)
	for rows.Next() {
		var value Assignment
		if err := rows.Scan(&value.ID, &value.Axis, &value.TargetID, &value.TierID, &value.TierVersion, &value.Actor, &value.Reason, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

// ModeChange changes a default or tier-specific step mode in a new policy version.
type ModeChange struct {
	StepKey string      `json:"step_key"`
	Axis    string      `json:"axis"`
	TierID  string      `json:"tier_id"`
	Mode    policy.Mode `json:"mode"`
}
