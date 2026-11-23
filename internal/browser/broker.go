// Package browser defines the durable boundary for harness tasks.
package browser

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"karun.tech/jobagent/internal/policy"
)

type BrowserTaskID int64
type BrowserTaskStatus string

const (
	Queued    BrowserTaskStatus = "QUEUED"
	Cancelled BrowserTaskStatus = "CANCELLED"
)

// BrowserTask contains only the information needed to queue an authorised read.
type BrowserTask struct {
	WorkflowID       string
	StepRunID        string
	Source           string
	Origin           string
	AllowedOrigins   []string
	Action           string
	AllowedActions   []string
	PolicySnapshotID int64
	PolicyVersion    int64
	Deadline         time.Time
	CallbackID       string
}

// BrowserTaskRunner is the host-facing task interface.
type BrowserTaskRunner interface {
	Dispatch(context.Context, BrowserTask) (BrowserTaskID, error)
	Status(context.Context, BrowserTaskID) (BrowserTaskStatus, error)
	Cancel(context.Context, BrowserTaskID) error
}

// Broker persists requests. It never launches a browser or invokes a harness.
type Broker struct{ DB *sql.DB }

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func validOrigin(origin string) bool {
	u, err := url.Parse(origin)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil &&
		u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/")
}

// Dispatch validates an authorised read task and durably queues it.
func (b Broker) Dispatch(ctx context.Context, task BrowserTask) (BrowserTaskID, error) {
	if task.WorkflowID == "" || task.StepRunID == "" || task.Source == "" ||
		task.CallbackID == "" || task.PolicySnapshotID <= 0 || task.PolicyVersion <= 0 ||
		!task.Deadline.After(time.Now()) || !validOrigin(task.Origin) ||
		!contains(task.AllowedOrigins, task.Origin) || !contains(task.AllowedActions, task.Action) {
		return 0, errors.New("invalid or unauthorised browser task")
	}
	if task.Action != "NAVIGATE" && task.Action != "READ" {
		return 0, errors.New("browser action is unavailable")
	}
	if err := policy.CheckSource(ctx, b.DB, task.Source, policy.Discovery); err != nil {
		return 0, fmt.Errorf("source permission: %w", err)
	}
	var originAllowed bool
	err := b.DB.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM source_browser_origins WHERE source = $1 AND origin = $2)",
		task.Source, strings.TrimSuffix(task.Origin, "/")).Scan(&originAllowed)
	if err != nil {
		return 0, fmt.Errorf("read source origins: %w", err)
	}
	if !originAllowed {
		return 0, errors.New("origin is not registered for this source")
	}
	var mode string
	var version int64
	var step string
	err = b.DB.QueryRowContext(ctx, `SELECT mode, policy_version, step_key
		FROM effective_step_policies WHERE id = $1`, task.PolicySnapshotID).Scan(&mode, &version, &step)
	if err != nil {
		return 0, fmt.Errorf("read policy snapshot: %w", err)
	}
	if mode == string(policy.Off) || version != task.PolicyVersion ||
		(step != "discover_company" && step != "validate_board") {
		return 0, errors.New("policy does not permit browser discovery")
	}
	var id BrowserTaskID
	err = b.DB.QueryRowContext(ctx, `INSERT INTO browser_tasks
		(workflow_id, step_run_id, source, origin, action, policy_snapshot_id, policy_version, deadline, callback_id, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'QUEUED') RETURNING id`,
		task.WorkflowID, task.StepRunID, task.Source, strings.TrimSuffix(task.Origin, "/"), task.Action,
		task.PolicySnapshotID, task.PolicyVersion, task.Deadline, task.CallbackID).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("queue browser task: %w", err)
	}
	return id, nil
}

// Status reads the durable state of a queued task.
func (b Broker) Status(ctx context.Context, id BrowserTaskID) (BrowserTaskStatus, error) {
	var status BrowserTaskStatus
	err := b.DB.QueryRowContext(ctx, "SELECT status FROM browser_tasks WHERE id = $1", id).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("read browser task: %w", err)
	}
	return status, nil
}

// Cancel cancels a queued task. Repeated cancellation is harmless.
func (b Broker) Cancel(ctx context.Context, id BrowserTaskID) error {
	result, err := b.DB.ExecContext(ctx, "UPDATE browser_tasks SET status = 'CANCELLED' WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("cancel browser task: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// StubRunner is a fail-closed adapter when no harness is configured.
type StubRunner struct{}

func (StubRunner) Dispatch(context.Context, BrowserTask) (BrowserTaskID, error) {
	return 0, errors.New("browser harness is not configured")
}
func (StubRunner) Status(context.Context, BrowserTaskID) (BrowserTaskStatus, error) {
	return "", errors.New("browser harness is not configured")
}
func (StubRunner) Cancel(context.Context, BrowserTaskID) error {
	return errors.New("browser harness is not configured")
}
