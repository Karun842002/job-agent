package policy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSourcePolicyIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	source := fmt.Sprintf("test-source-policy-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM source_policies WHERE source = $1", source)
		db.Close()
	})
	ctx := context.Background()
	if err := CheckSource(ctx, db, source, Discovery); !errors.Is(err, ErrSourceUnregistered) {
		t.Fatalf("missing source: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO source_policies
		(source, discovery_allowed, evidence, last_reviewed_at)
		VALUES ($1, true, 'reviewed fixture', now())`, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		capability Capability
		want       error
	}{
		{Discovery, nil},
		{Poll, ErrCapabilityDenied},
		{APISubmit, ErrCapabilityDenied},
		{BrowserSubmit, ErrCapabilityDenied},
		{"invalid", ErrUnknownCapability},
	} {
		if err := CheckSource(ctx, db, source, tc.capability); !errors.Is(err, tc.want) {
			t.Errorf("capability %s: got %v, want %v", tc.capability, err, tc.want)
		}
	}
	_, err = db.ExecContext(ctx, `UPDATE source_policies SET poll_allowed = true,
		api_submit_allowed = true, browser_submit_allowed = true WHERE source = $1`, source)
	if err != nil {
		t.Fatal(err)
	}
	for _, capability := range []Capability{Discovery, Poll, APISubmit, BrowserSubmit} {
		if err := CheckSource(ctx, db, source, capability); err != nil {
			t.Errorf("allowed %s: %v", capability, err)
		}
	}
	closedDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := closedDB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := CheckSource(ctx, closedDB, source, Discovery); err == nil {
		t.Fatal("closed database must fail closed")
	}
}
