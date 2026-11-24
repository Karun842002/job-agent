package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrIntakeDenied indicates the effective tier policy forbids intake.
var ErrIntakeDenied = errors.New("tier policy denies intake")

// ErrPolicyChanged indicates policy became more restrictive during an operation.
var ErrPolicyChanged = errors.New("policy changed during intake")

// ResolveIntakeTx resolves and records the latest policy in the caller's
// transaction. The advisory lock serializes policy changes with callers that
// use the same transaction to commit an allowed domain mutation.
func ResolveIntakeTx(ctx context.Context, tx *sql.Tx, companyID int64, step string, sourceAllowed bool) (int64, Decision, Mode, error) {
	if companyID <= 0 || (step != "discover_company" && step != "validate_board" && step != "tier_proposal") {
		return 0, Decision{Mode: Off}, Off, ErrIntakeDenied
	}
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(1948737201)"); err != nil {
		return 0, Decision{}, Off, err
	}
	var version, companyVersion, roleVersion int64
	var global, companyEnabled, roleEnabled bool
	var companyTier, companyMode, roleMode, defaultMode string
	err := tx.QueryRowContext(ctx, `SELECT p.version,p.global_enabled,c.company_tier_id,ct.enabled,rt.enabled,ct.version,rt.version,
		COALESCE(cd.mode,'OFF'),COALESCE(cm.mode,cd.mode,'OFF'),COALESCE(rm.mode,cd.mode,'OFF')
		FROM policy_versions p
		JOIN companies c ON c.id=$1
		JOIN policy_tiers ct ON ct.axis='company' AND ct.id=c.company_tier_id
		JOIN policy_tiers rt ON rt.axis='role' AND rt.id='UNASSIGNED'
		LEFT JOIN step_defaults cd ON cd.policy_version=p.version AND cd.step_key=$2
		LEFT JOIN tier_step_modes cm ON cm.policy_version=p.version AND cm.axis='company' AND cm.tier_id=ct.id AND cm.step_key=$2
		LEFT JOIN tier_step_modes rm ON rm.policy_version=p.version AND rm.axis='role' AND rm.tier_id=rt.id AND rm.step_key=$2
		ORDER BY p.version DESC LIMIT 1`, companyID, step).Scan(&version, &global, &companyTier, &companyEnabled, &roleEnabled, &companyVersion, &roleVersion, &defaultMode, &companyMode, &roleMode)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Decision{Mode: Off}, Off, ErrIntakeDenied
	}
	if err != nil {
		return 0, Decision{}, Off, fmt.Errorf("load intake policy: %w", err)
	}
	input := StepInput{StepKey: step, PolicyVersion: version, GlobalEnabled: global, SourceAllowed: sourceAllowed,
		Company: Axis{TierID: companyTier, Enabled: companyEnabled, Version: companyVersion, Mode: Mode(companyMode)},
		Role:    Axis{TierID: "UNASSIGNED", Enabled: roleEnabled, Version: roleVersion, Mode: Mode(roleMode)}, Default: Mode(defaultMode)}
	id, decision, err := RecordStep(ctx, tx, input)
	if err != nil {
		return 0, decision, Off, err
	}
	input.SourceAllowed = true
	intent, err := ResolveStep(input)
	if err != nil {
		return 0, Decision{}, Off, err
	}
	return id, decision, intent.Mode, nil
}

// ResolveIntake records a policy decision in its own transaction.
func ResolveIntake(ctx context.Context, db *sql.DB, companyID int64, step string, sourceAllowed bool) (int64, Decision, Mode, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, Decision{}, Off, err
	}
	defer tx.Rollback()
	id, decision, intent, err := ResolveIntakeTx(ctx, tx, companyID, step, sourceAllowed)
	if err != nil {
		return 0, decision, intent, err
	}
	if err := tx.Commit(); err != nil {
		return 0, Decision{}, Off, err
	}
	return id, decision, intent, nil
}
