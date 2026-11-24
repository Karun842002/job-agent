package tiers

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

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
