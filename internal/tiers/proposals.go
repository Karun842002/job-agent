package tiers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"karun.tech/jobagent/internal/audit"
	"karun.tech/jobagent/internal/policy"
)

// ProposalInput is deterministic evidence supplied by an authorized intake source.
// It contains no model output or raw candidate profile.
type ProposalInput struct {
	CompanyID    int64
	RoleFamilyID *int64
	SourceURL    string
	evidence     RelevanceEvidence
	ExpiresAt    time.Time
}

func validateProposalInput(in ProposalInput) (string, error) {
	if in.CompanyID <= 0 || (in.RoleFamilyID != nil && *in.RoleFamilyID <= 0) || !in.ExpiresAt.After(time.Now()) {
		return "", ErrInvalid
	}
	u, err := url.Parse(in.SourceURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return "", ErrInvalid
	}
	return strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")), nil
}

// propose is the internal policy-boundary helper for evaluated evidence.
func (s Store) propose(ctx context.Context, in ProposalInput) (int64, bool, error) {
	host, err := validateProposalInput(in)
	if err != nil {
		return 0, false, err
	}
	if !Relevant(in.evidence) {
		return 0, false, nil
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	id, created, err := proposeTx(ctx, tx, in, host)
	if err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return id, created, nil
}

// ProposeObservation reads verified resume evidence and source reliability,
// evaluates a structured observation, and writes its proposal atomically.
// Serialization failures are retried because no external effects occur here.
func (s Store) ProposeObservation(ctx context.Context, in ProposalInput, observed Observation) (int64, bool, error) {
	host, err := validateProposalInput(in)
	if err != nil {
		return 0, false, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		id, created, err := s.proposeObservationOnce(ctx, in, observed, host)
		var pgerr *pgconn.PgError
		if err == nil || !errors.As(err, &pgerr) || pgerr.Code != "40001" {
			return id, created, err
		}
	}
	return 0, false, errors.New("proposal could not be serialized after three attempts")
}

func (s Store) proposeObservationOnce(ctx context.Context, in ProposalInput, observed Observation, host string) (int64, bool, error) {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	facts, err := loadKnowledge(ctx, tx)
	if err != nil {
		return 0, false, err
	}
	reliability, err := loadSourceReliability(ctx, tx, host)
	if err != nil {
		return 0, false, err
	}
	in.evidence = EvaluateObservation(facts, observed, reliability)
	if !Relevant(in.evidence) {
		return 0, false, tx.Commit()
	}
	id, created, err := proposeTx(ctx, tx, in, host)
	if err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return id, created, nil
}

func proposeTx(ctx context.Context, tx *sql.Tx, in ProposalInput, host string) (int64, bool, error) {
	if err := policy.CheckSourceTx(ctx, tx, host, policy.Discovery); err != nil {
		return 0, false, err
	}
	snapshot, decision, _, err := policy.ResolveIntakeTx(ctx, tx, in.CompanyID, "tier_proposal", true)
	if err != nil {
		return 0, false, err
	}
	if decision.Mode == policy.Off {
		return 0, false, policy.ErrIntakeDenied
	}
	if in.RoleFamilyID == nil {
		var tier string
		if err := tx.QueryRowContext(ctx, "SELECT company_tier_id FROM companies WHERE id=$1 FOR SHARE", in.CompanyID).Scan(&tier); err != nil {
			return 0, false, err
		}
		if tier != "UNASSIGNED" {
			return 0, false, policy.ErrIntakeDenied
		}
	} else {
		var tier string
		var enabled bool
		if err := tx.QueryRowContext(ctx, `SELECT f.role_tier_id,t.enabled FROM role_families f
			JOIN policy_tiers t ON t.axis='role' AND t.id=f.role_tier_id WHERE f.id=$1 FOR SHARE OF f,t`, *in.RoleFamilyID).Scan(&tier, &enabled); err != nil {
			return 0, false, err
		}
		if !enabled || tier != "UNASSIGNED" {
			return 0, false, policy.ErrIntakeDenied
		}
	}
	evidence, err := json.Marshal(in.evidence)
	if err != nil {
		return 0, false, err
	}
	if decision.Mode == policy.Audit {
		diff, err := json.Marshal(map[string]any{"company_id": in.CompanyID, "role_family_id": in.RoleFamilyID, "would_create_tier_proposal": true})
		if err != nil {
			return 0, false, err
		}
		id, err := audit.SavePreviewTx(ctx, tx, audit.Preview{PolicySnapshotID: snapshot, StepRunID: fmt.Sprintf("proposal:%d", in.CompanyID), Diff: diff})
		if err != nil {
			return 0, false, err
		}
		return id, false, nil
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO tier_proposals(company_id,role_family_id,evidence,source_url,explanation,expires_at)
		VALUES($1,$2,$3,$4,'deterministic relevance checks passed',$5) ON CONFLICT DO NOTHING RETURNING id`,
		in.CompanyID, in.RoleFamilyID, evidence, in.SourceURL, in.ExpiresAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if _, err := audit.Append(ctx, tx, audit.Event{Name: "tier_proposed", Actor: "system", Cause: "deterministic_relevance", PolicySnapshotID: &snapshot, CompanyID: &in.CompanyID}); err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// Proposal is a durable tier review prompt.
type Proposal struct {
	ID              int64           `json:"id"`
	CompanyID       int64           `json:"company_id"`
	RoleFamilyID    *int64          `json:"role_family_id,omitempty"`
	Evidence        json.RawMessage `json:"evidence"`
	SourceURL       string          `json:"source_url"`
	SuggestedTierID *string         `json:"suggested_tier_id,omitempty"`
	Explanation     string          `json:"explanation"`
	Confidence      *float64        `json:"confidence,omitempty"`
	Status          string          `json:"status"`
	NotifiedAt      *time.Time      `json:"notified_at,omitempty"`
	DecisionActor   *string         `json:"decision_actor,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
}

// ListProposals returns active review prompts in stable ID order.
func (s Store) ListProposals(ctx context.Context, after int64) ([]Proposal, error) {
	if after < 0 {
		return nil, ErrInvalid
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,company_id,role_family_id,evidence,source_url,suggested_tier_id,
		explanation,confidence,status,notified_at,decision_actor,created_at,expires_at
		FROM tier_proposals WHERE id>$1 AND status IN ('PENDING','SNOOZED') AND expires_at>now()
		ORDER BY id LIMIT 100`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Proposal, 0)
	for rows.Next() {
		var p Proposal
		var role sql.NullInt64
		var tier, actor sql.NullString
		var confidence sql.NullFloat64
		var notified sql.NullTime
		if err := rows.Scan(&p.ID, &p.CompanyID, &role, &p.Evidence, &p.SourceURL, &tier,
			&p.Explanation, &confidence, &p.Status, &notified, &actor, &p.CreatedAt, &p.ExpiresAt); err != nil {
			return nil, err
		}
		if role.Valid {
			p.RoleFamilyID = &role.Int64
		}
		if tier.Valid {
			p.SuggestedTierID = &tier.String
		}
		if actor.Valid {
			p.DecisionActor = &actor.String
		}
		if confidence.Valid {
			p.Confidence = &confidence.Float64
		}
		if notified.Valid {
			p.NotifiedAt = &notified.Time
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
