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

	"karun.tech/jobagent/internal/audit"
	"karun.tech/jobagent/internal/policy"
)

// ProposalInput is deterministic evidence supplied by an authorized intake source.
// It contains no model output or raw candidate profile.
type ProposalInput struct {
	CompanyID    int64
	RoleFamilyID *int64
	SourceURL    string
	Evidence     RelevanceEvidence
	ExpiresAt    time.Time
}

// Propose applies the relevance rule and records a single prompt per identity.
// AUDIT stores only a preview and OFF rejects the operation. Evidence collection
// remains the caller's responsibility; source permissions are checked here.
func (s Store) Propose(ctx context.Context, in ProposalInput) (int64, bool, error) {
	if in.CompanyID <= 0 || (in.RoleFamilyID != nil && *in.RoleFamilyID <= 0) || !in.ExpiresAt.After(time.Now()) {
		return 0, false, ErrInvalid
	}
	u, err := url.Parse(in.SourceURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" {
		return 0, false, ErrInvalid
	}
	if !Relevant(in.Evidence) {
		return 0, false, nil
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
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
	evidence, err := json.Marshal(in.Evidence)
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
		return id, false, tx.Commit()
	}
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO tier_proposals(company_id,role_family_id,evidence,source_url,explanation,expires_at)
		VALUES($1,$2,$3,$4,'deterministic relevance checks passed',$5) ON CONFLICT DO NOTHING RETURNING id`,
		in.CompanyID, in.RoleFamilyID, evidence, in.SourceURL, in.ExpiresAt).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, tx.Commit()
	}
	if err != nil {
		return 0, false, err
	}
	if _, err := audit.Append(ctx, tx, audit.Event{Name: "tier_proposed", Actor: "system", Cause: "deterministic_relevance", PolicySnapshotID: &snapshot, CompanyID: &in.CompanyID}); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
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
