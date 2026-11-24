package company

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

func jsonObject(data json.RawMessage) bool {
	if len(data) == 0 || len(data) > 65536 {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(data, &object) == nil && object != nil
}

// SetBoardMetadata records observed board data and separate operator overrides.
// Source permissions are evaluated separately and cannot be granted here.
func (s Store) SetBoardMetadata(ctx context.Context, id int64, value BoardMetadata) error {
	switch value.State {
	case "candidate", "verified", "degraded", "inactive", "blocked_by_policy", "manual_review":
	default:
		return fmt.Errorf("%w: invalid board state", ErrInvalidInput)
	}
	if id <= 0 || !jsonObject(value.Capabilities) || !jsonObject(value.Provenance) || !jsonObject(value.OperatorOverride) {
		return fmt.Errorf("%w: metadata objects and board ID are required", ErrInvalidInput)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var companyID int64
	err = tx.QueryRowContext(ctx, `UPDATE ats_boards SET state=$2,capabilities=$3,
		provenance=$4,last_verified_at=$5,operator_override=$6
		WHERE id=$1 RETURNING company_id`, id, value.State, value.Capabilities,
		value.Provenance, value.LastVerifiedAt, value.OperatorOverride).Scan(&companyID)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("update board metadata: %w", err)
	}
	if err := auditMutation(ctx, tx, "board_metadata_updated", companyID); err != nil {
		return err
	}
	return tx.Commit()
}
