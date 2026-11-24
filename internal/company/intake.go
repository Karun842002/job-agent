package company

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"karun.tech/jobagent/internal/audit"
	"karun.tech/jobagent/internal/policy"
)

type intakeGuard struct {
	db         *sql.DB
	companyID  int64
	step       string
	sources    []string
	snapshotID int64
	mode       policy.Mode
}

func sourceDenial(err error) bool {
	return errors.Is(err, policy.ErrCapabilityDenied) || errors.Is(err, policy.ErrSourceUnregistered)
}

func startIntake(ctx context.Context, db *sql.DB, companyID int64, step string, sources ...string) (*intakeGuard, error) {
	var denied error
	for _, source := range sources {
		if err := policy.CheckSource(ctx, db, source, policy.Discovery); err != nil {
			if !sourceDenial(err) {
				return nil, err
			}
			if denied == nil {
				denied = err
			}
		}
	}
	id, decision, intent, err := policy.ResolveIntake(ctx, db, companyID, step, denied == nil)
	if err != nil {
		return nil, err
	}
	guard := &intakeGuard{db: db, companyID: companyID, step: step, sources: sources, snapshotID: id, mode: intent}
	if intent == policy.Off {
		return guard, policy.ErrIntakeDenied
	}
	if denied != nil {
		return guard, denied
	}
	guard.mode = decision.Mode
	return guard, nil
}

func (g *intakeGuard) authorize(ctx context.Context, host string) error {
	var denied error
	sources := append(append([]string{}, g.sources...), host)
	for _, source := range sources {
		if err := policy.CheckSource(ctx, g.db, source, policy.Discovery); err != nil {
			if !sourceDenial(err) {
				return err
			}
			if denied == nil {
				denied = err
			}
		}
	}
	_, decision, intent, err := policy.ResolveIntake(ctx, g.db, g.companyID, g.step, denied == nil)
	if err != nil {
		return err
	}
	if intent == policy.Off || (g.mode == policy.Live && intent != policy.Live) {
		return policy.ErrPolicyChanged
	}
	if denied != nil {
		return denied
	}
	if decision.Mode == policy.Off || (g.mode == policy.Live && decision.Mode != policy.Live) {
		return policy.ErrPolicyChanged
	}
	return nil
}

func (g *intakeGuard) checkCommit(ctx context.Context, tx *sql.Tx, host string) error {
	sources := append(append([]string{}, g.sources...), host)
	for _, source := range sources {
		if err := policy.CheckSourceTx(ctx, tx, source, policy.Discovery); err != nil {
			return err
		}
	}
	_, decision, _, err := policy.ResolveIntakeTx(ctx, tx, g.companyID, g.step, true)
	if err != nil {
		return err
	}
	if decision.Mode != policy.Live {
		return policy.ErrPolicyChanged
	}
	return nil
}

func (g *intakeGuard) checkIntentCommit(ctx context.Context, tx *sql.Tx, sourceAllowed bool) error {
	_, _, intent, err := policy.ResolveIntakeTx(ctx, tx, g.companyID, g.step, sourceAllowed)
	if err != nil {
		return err
	}
	if intent != policy.Live {
		return policy.ErrPolicyChanged
	}
	return nil
}

func (g *intakeGuard) preview(ctx context.Context, stepID string, value any) error {
	if g.mode != policy.Audit {
		return errors.New("audit preview requires audit mode")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tx, err := g.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, _, intent, err := policy.ResolveIntakeTx(ctx, tx, g.companyID, g.step, true)
	if err != nil {
		return err
	}
	if intent == policy.Off {
		return policy.ErrPolicyChanged
	}
	_, err = audit.SavePreviewTx(ctx, tx, audit.Preview{PolicySnapshotID: g.snapshotID, StepRunID: stepID, Diff: data})
	if err != nil {
		return fmt.Errorf("save intake preview: %w", err)
	}
	return tx.Commit()
}
