// Package audit persists transactionally correlated audit events.
package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

// Event contains metadata and an opaque evidence identifier, never raw evidence.
type Event struct {
	Name             string
	Actor            string
	Cause            string
	PolicySnapshotID *int64
	TraceID          *string
	StepRunID        *string
	EvidenceRef      *string
	CompanyID        *int64
	JobID            *int64
	ApplicationID    *int64
	BeforeHash       *string
	AfterHash        *string
}

var metadataToken = regexp.MustCompile(`^[a-zA-Z0-9_:-]{1,128}$`)

// Append writes an event within the caller's transaction.
func Append(ctx context.Context, tx *sql.Tx, event Event) (int64, error) {
	if !metadataToken.MatchString(event.Name) || !metadataToken.MatchString(event.Actor) || !metadataToken.MatchString(event.Cause) {
		return 0, errors.New("event name, actor and cause must be metadata tokens")
	}
	for _, value := range []*string{event.TraceID, event.StepRunID} {
		if value != nil && !metadataToken.MatchString(*value) {
			return 0, errors.New("correlation IDs must be metadata tokens")
		}
	}
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO audit_events
		(event_name, actor, cause, policy_snapshot_id, trace_id, step_run_id, evidence_ref,
		company_id, job_id, application_id, before_hash, after_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`, event.Name, event.Actor, event.Cause,
		event.PolicySnapshotID, event.TraceID, event.StepRunID, event.EvidenceRef,
		event.CompanyID, event.JobID, event.ApplicationID, event.BeforeHash, event.AfterHash).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("append audit event: %w", err)
	}
	return id, nil
}
