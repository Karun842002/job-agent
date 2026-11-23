package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Preview describes hypothetical changes. It is stored separately from domain state.
type Preview struct {
	PolicySnapshotID int64
	StepRunID        string
	Diff             json.RawMessage
}

// SavePreview only persists a hypothetical diff for an AUDIT policy snapshot.
// It does not accept a browser, network client, or domain-state writer.
func SavePreview(ctx context.Context, db *sql.DB, preview Preview) (int64, error) {
	if preview.PolicySnapshotID <= 0 || !metadataToken.MatchString(preview.StepRunID) ||
		len(preview.Diff) == 0 || len(preview.Diff) > 65536 || !json.Valid(preview.Diff) {
		return 0, errors.New("invalid audit preview")
	}
	var id int64
	err := db.QueryRowContext(ctx, `INSERT INTO audit_previews
		(policy_snapshot_id, step_run_id, diff)
		SELECT id, $2, $3 FROM effective_step_policies
		WHERE id = $1 AND mode = 'AUDIT'
		RETURNING id`, preview.PolicySnapshotID, preview.StepRunID, preview.Diff).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errors.New("audit policy snapshot is required")
	}
	if err != nil {
		return 0, fmt.Errorf("store audit preview: %w", err)
	}
	return id, nil
}
