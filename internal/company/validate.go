package company

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"karun.tech/jobagent/internal/platform/httpclient"
	"karun.tech/jobagent/internal/policy"
)

// ValidationResult describes observed provider visibility without inferring closure.
type ValidationResult struct {
	Provider string `json:"provider"`
	Listed   *int   `json:"listed,omitempty"`
	Unlisted *int   `json:"unlisted,omitempty"`
	Unknown  *int   `json:"unknown,omitempty"`
}

// Validator checks public provider board endpoints under source policy.
type Validator struct {
	DB    *sql.DB
	fetch func(context.Context, string, []string, func(context.Context, string) error) (int, []byte, error)
}

// NewValidator creates a bounded policy-gated provider validator.
func NewValidator(db *sql.DB) *Validator {
	return &Validator{DB: db, fetch: func(ctx context.Context, target string, hosts []string, authorize func(context.Context, string) error) (int, []byte, error) {
		client, err := httpclient.New(hosts, authorize)
		if err != nil {
			return 0, nil, err
		}
		defer client.Close()
		return client.Get(ctx, target)
	}}
}

func (v *Validator) setResult(ctx context.Context, board Board, state string, capabilities []byte) error {
	tx, err := v.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var companyID int64
	err = tx.QueryRowContext(ctx, `UPDATE ats_boards SET state=$6,
		capabilities=CASE WHEN $7::jsonb IS NULL THEN capabilities ELSE capabilities || $7::jsonb END,
		last_verified_at=CASE WHEN $6='verified' THEN now() ELSE last_verified_at END
		WHERE id=$1 AND company_id=$2 AND provider=$3 AND region=$4 AND board_key=$5 RETURNING company_id`,
		board.ID, board.CompanyID, board.Provider, board.Region, board.Key, state, capabilities).Scan(&companyID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("board changed during validation")
	}
	if err != nil {
		return fmt.Errorf("update board validation: %w", err)
	}
	if err := auditMutation(ctx, tx, "board_validation_recorded", companyID); err != nil {
		return err
	}
	return tx.Commit()
}

// Validate checks a board and records verified, degraded or policy-blocked state.
func (v *Validator) Validate(ctx context.Context, id int64) (ValidationResult, error) {
	board, err := (Store{DB: v.DB}).GetBoard(ctx, id)
	if err != nil {
		return ValidationResult{}, err
	}
	if !boardKeyPattern.MatchString(board.Key) || board.Key == "." || board.Key == ".." {
		if updateErr := v.setResult(ctx, board, "degraded", nil); updateErr != nil {
			return ValidationResult{}, updateErr
		}
		return ValidationResult{}, errors.New("invalid board key")
	}
	var host, path string
	switch board.Provider {
	case "greenhouse":
		host = "boards-api.greenhouse.io"
		path = "/v1/boards/" + url.PathEscape(board.Key)
	case "ashby":
		host = "api.ashbyhq.com"
		path = "/posting-api/job-board/" + url.PathEscape(board.Key)
	default:
		if updateErr := v.setResult(ctx, board, "degraded", nil); updateErr != nil {
			return ValidationResult{}, updateErr
		}
		return ValidationResult{}, errors.New("unsupported board provider")
	}
	if err := policy.CheckSource(ctx, v.DB, host, policy.Discovery); err != nil {
		if errors.Is(err, policy.ErrCapabilityDenied) || errors.Is(err, policy.ErrSourceUnregistered) {
			if updateErr := v.setResult(ctx, board, "blocked_by_policy", nil); updateErr != nil {
				return ValidationResult{}, updateErr
			}
		}
		return ValidationResult{}, err
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	status, data, err := v.fetch(fetchCtx, (&url.URL{Scheme: "https", Host: host, Path: path}).String(), []string{host}, func(ctx context.Context, host string) error {
		return policy.CheckSource(ctx, v.DB, host, policy.Discovery)
	})
	if err != nil {
		state := "degraded"
		if errors.Is(err, policy.ErrCapabilityDenied) || errors.Is(err, policy.ErrSourceUnregistered) {
			state = "blocked_by_policy"
		}
		if updateErr := v.setResult(ctx, board, state, nil); updateErr != nil {
			return ValidationResult{}, updateErr
		}
		return ValidationResult{}, err
	}
	if status < 200 || status >= 300 {
		state := "degraded"
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			state = "blocked_by_policy"
		}
		if err := v.setResult(ctx, board, state, nil); err != nil {
			return ValidationResult{}, err
		}
		return ValidationResult{}, fmt.Errorf("provider returned status %d", status)
	}
	result := ValidationResult{Provider: board.Provider}
	switch board.Provider {
	case "greenhouse":
		var response struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &response); err != nil || response.Name == "" {
			if updateErr := v.setResult(ctx, board, "degraded", nil); updateErr != nil {
				return ValidationResult{}, updateErr
			}
			return ValidationResult{}, errors.New("invalid Greenhouse board response")
		}
	case "ashby":
		var response struct {
			Jobs []struct {
				IsListed *bool `json:"isListed"`
			} `json:"jobs"`
		}
		if err := json.Unmarshal(data, &response); err != nil || response.Jobs == nil {
			if updateErr := v.setResult(ctx, board, "degraded", nil); updateErr != nil {
				return ValidationResult{}, updateErr
			}
			return ValidationResult{}, errors.New("invalid Ashby board response")
		}
		listed, unlisted, unknown := 0, 0, 0
		for _, job := range response.Jobs {
			if job.IsListed == nil {
				unknown++
			} else if *job.IsListed {
				listed++
			} else {
				unlisted++
			}
		}
		result.Listed, result.Unlisted, result.Unknown = &listed, &unlisted, &unknown
	}
	observed := map[string]any{"public_read": true}
	if result.Listed != nil {
		observed["listed_jobs"] = *result.Listed
		observed["unlisted_jobs"] = *result.Unlisted
		observed["unknown_listing_jobs"] = *result.Unknown
	}
	capabilities, err := json.Marshal(observed)
	if err != nil {
		return ValidationResult{}, err
	}
	if err := v.setResult(ctx, board, "verified", capabilities); err != nil {
		return ValidationResult{}, err
	}
	return result, nil
}
