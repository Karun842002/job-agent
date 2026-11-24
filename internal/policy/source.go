package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Capability identifies an independently controlled source action.
type Capability string

const (
	Discovery     Capability = "discovery"
	Poll          Capability = "poll"
	APISubmit     Capability = "api_submit"
	BrowserSubmit Capability = "browser_submit"
)

var (
	ErrSourceUnregistered = errors.New("source policy is not registered")
	ErrCapabilityDenied   = errors.New("source capability is denied")
	ErrUnknownCapability  = errors.New("unknown source capability")
)

// CheckSource verifies the requested capability against the stored policy.
// Missing policies, denied actions, and storage failures all stop the action.
func CheckSource(ctx context.Context, db *sql.DB, source string, capability Capability) error {
	return checkSource(ctx, db, source, capability, false)
}

// CheckSourceTx checks and locks a source policy until the transaction ends.
func CheckSourceTx(ctx context.Context, tx *sql.Tx, source string, capability Capability) error {
	return checkSource(ctx, tx, source, capability, true)
}

type sourceQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func checkSource(ctx context.Context, db sourceQuerier, source string, capability Capability, lock bool) error {
	var discovery, poll, apiSubmit, browserSubmit bool
	query := `SELECT discovery_allowed, poll_allowed,
		api_submit_allowed, browser_submit_allowed FROM source_policies WHERE source = $1`
	if lock {
		query += " FOR SHARE"
	}
	err := db.QueryRowContext(ctx, query, source).Scan(&discovery, &poll, &apiSubmit, &browserSubmit)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: register and review the source before use", ErrSourceUnregistered)
	}
	if err != nil {
		return fmt.Errorf("read source policy: %w", err)
	}
	var allowed bool
	switch capability {
	case Discovery:
		allowed = discovery
	case Poll:
		allowed = poll
	case APISubmit:
		allowed = apiSubmit
	case BrowserSubmit:
		allowed = browserSubmit
	default:
		return ErrUnknownCapability
	}
	if !allowed {
		return fmt.Errorf("%w: review the %s capability before use", ErrCapabilityDenied, capability)
	}
	return nil
}
