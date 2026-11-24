package tiers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"karun.tech/jobagent/internal/policy"
)

func TestProposePolicyAndDeduplicationIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	host := "proposal-" + suffix + ".example"
	var company int64
	if err := db.QueryRowContext(ctx, `INSERT INTO companies(name,domain) VALUES('fixture',$1) RETURNING id`, host).Scan(&company); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM tier_proposals WHERE company_id=$1", company); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM companies WHERE id=$1", company); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM source_policies WHERE source=$1", host); err != nil {
			t.Error(err)
		}
	}()
	version := time.Now().UnixNano()
	if _, err := db.ExecContext(ctx, `INSERT INTO policy_versions(version,global_enabled) VALUES($1,true)`, version); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO step_defaults(policy_version,step_key,mode) VALUES($1,'tier_proposal','LIVE')`, version); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO source_policies(source,discovery_allowed) VALUES($1,true)`, host); err != nil {
		t.Fatal(err)
	}
	store := Store{DB: db}
	in := ProposalInput{CompanyID: company, SourceURL: "https://" + host + "/careers", evidence: RelevanceEvidence{Matches, Matches, Matches, Matches, NoMatch}, ExpiresAt: time.Now().Add(time.Hour)}
	type outcome struct {
		id      int64
		created bool
		err     error
	}
	results := make(chan outcome, 8)
	var wg sync.WaitGroup
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, created, err := store.propose(ctx, in)
			results <- outcome{id, created, err}
		}()
	}
	wg.Wait()
	close(results)
	createdCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent proposal: %v", result.err)
		}
		if result.created {
			createdCount++
			if result.id == 0 {
				t.Fatal("created proposal has no ID")
			}
		}
	}
	if createdCount != 1 {
		t.Fatalf("concurrent creation count=%d, want 1", createdCount)
	}
	var created bool
	_, created, err = store.propose(ctx, in)
	if err != nil || created {
		t.Fatalf("duplicate created=%v err=%v", created, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM tier_proposals WHERE company_id=$1", company).Scan(&count); err != nil || count != 1 {
		t.Fatalf("proposal count=%d err=%v", count, err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE source_policies SET discovery_allowed=false WHERE source=$1", host); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.propose(ctx, in); !errors.Is(err, policy.ErrCapabilityDenied) {
		t.Fatalf("denied source: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE source_policies SET discovery_allowed=true WHERE source=$1", host); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(ctx, ModeChange{StepKey: "tier_proposal", Mode: policy.Off}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.propose(ctx, in); !errors.Is(err, policy.ErrIntakeDenied) {
		t.Fatalf("OFF mode: %v", err)
	}
	if _, err := store.SetMode(ctx, ModeChange{StepKey: "tier_proposal", Mode: policy.Audit}); err != nil {
		t.Fatal(err)
	}
	preview, created, err := store.propose(ctx, in)
	if err != nil || created || preview == 0 {
		t.Fatalf("AUDIT preview=%d created=%v err=%v", preview, created, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM tier_proposals WHERE company_id=$1", company).Scan(&count); err != nil || count != 1 {
		t.Fatalf("AUDIT changed domain count=%d err=%v", count, err)
	}
}

func TestListProposalsIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var company, proposal int64
	domain := fmt.Sprintf("proposal-list-%d.example", time.Now().UnixNano())
	if err := db.QueryRowContext(ctx, `INSERT INTO companies(name,domain) VALUES('fixture',$1) RETURNING id`, domain).Scan(&company); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM tier_proposals WHERE company_id=$1", company); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM companies WHERE id=$1", company); err != nil {
			t.Error(err)
		}
	}()
	if err := db.QueryRowContext(ctx, `INSERT INTO tier_proposals(company_id,evidence,source_url,explanation,expires_at)
		VALUES($1,'{}','https://example.com','fixture',now()+interval '1 hour') RETURNING id`, company).Scan(&proposal); err != nil {
		t.Fatal(err)
	}
	items, err := (Store{DB: db}).ListProposals(ctx, proposal-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].ID != proposal || items[0].CompanyID != company || items[0].Status != "PENDING" {
		t.Fatalf("unexpected proposals: %+v", items)
	}
	items, err = (Store{DB: db}).ListProposals(ctx, proposal)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == proposal {
			t.Fatal("pagination repeated proposal")
		}
	}
	if _, err := (Store{DB: db}).ListProposals(ctx, -1); err != ErrInvalid {
		t.Fatalf("negative cursor: %v", err)
	}
}
