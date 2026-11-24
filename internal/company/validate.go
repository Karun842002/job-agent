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

func (v *Validator) setResult(ctx context.Context, board Board, state string, capabilities []byte, guard *intakeGuard, host string, sourceAllowed bool) error {
	tx, err := v.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if sourceAllowed {
		err = guard.checkCommit(ctx, tx, host)
	} else {
		err = guard.checkIntentCommit(ctx, tx, false)
	}
	if err != nil {
		return err
	}
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

func validationSourceDenied(err error) bool {
	return sourceDenial(err) || errors.Is(err, policy.ErrIntakeDenied)
}

func (v *Validator) recordOutcome(ctx context.Context, board Board, guard *intakeGuard, host, outcome string, sourceAllowed bool) error {
	if guard.mode == policy.Audit {
		return guard.preview(ctx, fmt.Sprintf("board-%d", board.ID), map[string]any{"outcome": outcome})
	}
	if guard.mode != policy.Live {
		return nil
	}
	state := "degraded"
	if outcome == "blocked_by_policy" {
		state = outcome
	}
	return v.setResult(ctx, board, state, nil, guard, host, sourceAllowed)
}

// Validate checks a board and records verified, degraded or policy-blocked state.
func (v *Validator) Validate(ctx context.Context, id int64) (ValidationResult, error) {
	board, err := (Store{DB: v.DB}).GetBoard(ctx, id)
	if err != nil {
		return ValidationResult{}, err
	}
	var host, path string
	switch board.Provider {
	case "greenhouse":
		host, path = "boards-api.greenhouse.io", "/v1/boards/"+url.PathEscape(board.Key)
	case "ashby":
		host, path = "api.ashbyhq.com", "/posting-api/job-board/"+url.PathEscape(board.Key)
	default:
		return ValidationResult{}, errors.New("unsupported board provider")
	}
	if !boardKeyPattern.MatchString(board.Key) || board.Key == "." || board.Key == ".." {
		return ValidationResult{}, errors.New("invalid board key")
	}
	boardScope := "board:" + board.Provider + ":" + board.Key
	guard, err := startIntake(ctx, v.DB, board.CompanyID, "validate_board", boardScope, host)
	if err != nil {
		if guard != nil && guard.mode == policy.Live && validationSourceDenied(err) {
			if updateErr := v.recordOutcome(ctx, board, guard, host, "blocked_by_policy", false); updateErr != nil {
				return ValidationResult{}, updateErr
			}
		}
		return ValidationResult{}, err
	}
	if guard.mode == policy.Audit {
		// AUDIT is observational only. Fetch and return facts, but never alter board state.
	}
	authorize := func(ctx context.Context, requestHost string) error {
		if requestHost != host {
			return policy.ErrCapabilityDenied
		}
		return guard.authorize(ctx, requestHost)
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	status, data, err := v.fetch(fetchCtx, (&url.URL{Scheme: "https", Host: host, Path: path}).String(), []string{host}, authorize)
	if err != nil {
		if guard.mode == policy.Audit {
			if previewErr := v.recordOutcome(ctx, board, guard, host, "blocked_or_failed", true); previewErr != nil {
				return ValidationResult{}, previewErr
			}
		} else if validationSourceDenied(err) {
			if updateErr := v.recordOutcome(ctx, board, guard, host, "blocked_by_policy", false); updateErr != nil {
				return ValidationResult{}, updateErr
			}
		} else if guard.mode == policy.Live {
			if updateErr := v.recordOutcome(ctx, board, guard, host, "degraded", true); updateErr != nil {
				return ValidationResult{}, updateErr
			}
		}
		return ValidationResult{}, err
	}
	if status < 200 || status >= 300 {
		outcome := "degraded"
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			outcome = "blocked_by_policy"
		}
		if err := v.recordOutcome(ctx, board, guard, host, outcome, true); err != nil {
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
			if recordErr := v.recordOutcome(ctx, board, guard, host, "degraded", true); recordErr != nil {
				return ValidationResult{}, recordErr
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
			if recordErr := v.recordOutcome(ctx, board, guard, host, "degraded", true); recordErr != nil {
				return ValidationResult{}, recordErr
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
	if guard.mode == policy.Audit {
		if err := guard.preview(ctx, fmt.Sprintf("board-%d", board.ID), map[string]any{"result": result}); err != nil {
			return ValidationResult{}, err
		}
		return result, nil
	}
	observed := map[string]any{"public_read": true}
	if result.Listed != nil {
		observed["listed_jobs"], observed["unlisted_jobs"], observed["unknown_listing_jobs"] = *result.Listed, *result.Unlisted, *result.Unknown
	}
	capabilities, err := json.Marshal(observed)
	if err != nil {
		return ValidationResult{}, err
	}
	if err := v.setResult(ctx, board, "verified", capabilities, guard, host, true); err != nil {
		return ValidationResult{}, err
	}
	return result, nil
}
