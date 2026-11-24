package policy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Mode controls the external and domain effects of a workflow step.
type Mode string

const (
	Live  Mode = "LIVE"
	Audit Mode = "AUDIT"
	Off   Mode = "OFF"
)

// Axis contains the policy for one tier axis. An empty mode uses the default.
type Axis struct {
	TierID  string
	Enabled bool
	Mode    Mode
}

// StepInput is the complete policy state captured for a decision.
type StepInput struct {
	StepKey            string
	PolicyVersion      int64
	GlobalEnabled      bool
	SourceAllowed      bool
	Company            Axis
	Role               Axis
	Default            Mode
	HardLimit          Mode
	JobOverride        Mode
	SubmissionApproved bool
}

// Decision is the resolved mode and an explanation suitable for audit.
type Decision struct {
	Mode   Mode
	Reason string
}

func validMode(mode Mode) bool {
	return mode == Live || mode == Audit || mode == Off
}

func restrict(left, right Mode) Mode {
	if left == Off || right == Off {
		return Off
	}
	if left == Audit || right == Audit {
		return Audit
	}
	return Live
}

// ResolveStep applies global, tier and per-step restrictions.
func ResolveStep(input StepInput) (Decision, error) {
	if input.StepKey == "" || input.PolicyVersion <= 0 || !validMode(input.Default) {
		return Decision{Off, "invalid policy"}, errors.New("step, policy version and valid default are required")
	}
	for _, mode := range []Mode{input.Company.Mode, input.Role.Mode, input.HardLimit, input.JobOverride} {
		if mode != "" && !validMode(mode) {
			return Decision{Off, "invalid policy"}, fmt.Errorf("invalid mode %q", mode)
		}
	}
	if !input.GlobalEnabled {
		return Decision{Off, "global emergency stop"}, nil
	}
	if !input.SourceAllowed {
		return Decision{Off, "source capability denied"}, nil
	}
	if !input.Company.Enabled || !input.Role.Enabled {
		return Decision{Off, "tier disabled"}, nil
	}
	if input.Company.TierID == "" || input.Role.TierID == "" {
		return Decision{Off, "tier assignment missing"}, nil
	}
	if input.Company.TierID == "EXCLUDED" || input.Role.TierID == "EXCLUDED" {
		return Decision{Off, "tier excluded"}, nil
	}
	unassigned := input.Company.TierID == "UNASSIGNED" || input.Role.TierID == "UNASSIGNED"
	if unassigned && input.StepKey != "discover_company" && input.StepKey != "tier_proposal" && input.StepKey != "validate_board" {
		return Decision{Off, "unassigned tier is review-only"}, nil
	}
	companyMode, roleMode := input.Company.Mode, input.Role.Mode
	if companyMode == "" {
		companyMode = input.Default
	}
	if roleMode == "" {
		roleMode = input.Default
	}
	mode := restrict(companyMode, roleMode)
	if input.HardLimit != "" {
		mode = restrict(mode, input.HardLimit)
	}
	if input.JobOverride != "" {
		mode = restrict(mode, input.JobOverride)
	}
	if input.StepKey == "submit_application" && !input.SubmissionApproved {
		return Decision{Off, "submission approval required"}, nil
	}
	return Decision{mode, "resolved tier and step restrictions"}, nil
}

// RecordStep resolves and persists the input and result as a policy snapshot.
func RecordStep(ctx context.Context, tx *sql.Tx, input StepInput) (int64, Decision, error) {
	decision, err := ResolveStep(input)
	if err != nil {
		return 0, decision, err
	}
	data, err := json.Marshal(input)
	if err != nil {
		return 0, Decision{}, fmt.Errorf("encode policy snapshot: %w", err)
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO effective_step_policies
		(step_key, policy_version, inputs, mode, reason) VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, input.StepKey, input.PolicyVersion, data, decision.Mode, decision.Reason).Scan(&id)
	if err != nil {
		return 0, Decision{}, fmt.Errorf("persist policy snapshot: %w", err)
	}
	return id, decision, nil
}

// ErrGlobalStopped indicates that no enabled global policy is available.
var ErrGlobalStopped = errors.New("global policy is disabled or unavailable")

// CheckGlobal fails closed when the latest policy is absent or stopped.
func CheckGlobal(ctx context.Context, db *sql.DB) error {
	var enabled bool
	err := db.QueryRowContext(ctx, "SELECT global_enabled FROM policy_versions ORDER BY version DESC LIMIT 1").Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !enabled) {
		return ErrGlobalStopped
	}
	if err != nil {
		return fmt.Errorf("read global policy: %w", err)
	}
	return nil
}
