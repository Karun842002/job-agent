package tiers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"karun.tech/jobagent/internal/policy"
)

func TestTierManagementIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	store := Store{DB: db}
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	tierID := "TEST_" + strings.ToUpper(suffix)
	version := time.Now().UnixNano()
	if _, err := db.ExecContext(ctx, "INSERT INTO policy_versions(version,global_enabled) VALUES($1,true)", version); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO step_defaults(policy_version,step_key,mode) VALUES($1,'validate_board','LIVE')", version); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"discover_company", "tier_proposal"} {
		if _, err := db.ExecContext(ctx, "INSERT INTO step_defaults(policy_version,step_key,mode) VALUES($1,$2,'LIVE')", version, step); err != nil {
			t.Fatal(err)
		}
	}
	var companyID int64
	if err := db.QueryRowContext(ctx, "INSERT INTO companies(name,domain) VALUES($1,$2) RETURNING id", "Tier fixture", "tier-"+suffix+".example").Scan(&companyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", databaseURL)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Close()
		if _, err := cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE id=$1", companyID); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.ExecContext(ctx, `INSERT INTO tier_assignments(axis,target_id,tier_id,tier_version,actor,reason)
		SELECT 'company',$1,id,version,'system','initial_default' FROM policy_tiers WHERE axis='company' AND id='UNASSIGNED'`, companyID); err != nil {
		t.Fatal(err)
	}
	created, err := store.SaveTier(ctx, Tier{ID: tierID, Axis: "company", Name: "Test tier", Enabled: true, Priority: 3, Settings: []byte(`{"poll_minutes":60}`)})
	if err != nil || created.Version != 1 {
		t.Fatalf("create tier: %+v %v", created, err)
	}
	if err := store.Assign(ctx, "company", companyID, tierID, "integration test"); err != nil {
		t.Fatal(err)
	}
	history, err := store.Assignments(ctx, "company", companyID, 0)
	if err != nil || len(history) != 2 || history[0].TierID != "UNASSIGNED" || history[1].TierID != tierID || history[1].TierVersion != 1 {
		t.Fatalf("assignment history: %+v %v", history, err)
	}
	snapshotID, decision, mode, err := policy.ResolveIntake(ctx, db, companyID, "validate_board", true)
	if err != nil || snapshotID == 0 || decision.Mode != policy.Live || mode != policy.Live {
		t.Fatalf("enabled policy: snapshot=%d decision=%+v mode=%s err=%v", snapshotID, decision, mode, err)
	}

	var familyID int64
	if err := db.QueryRowContext(ctx, "INSERT INTO role_families(name) VALUES($1) RETURNING id", "Fixture role").Scan(&familyID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO tier_assignments(axis,target_id,tier_id,tier_version,actor,reason)
		SELECT 'role',$1,id,version,'system','initial_default' FROM policy_tiers WHERE axis='role' AND id='UNASSIGNED'`, familyID); err != nil {
		t.Fatal(err)
	}
	if err := store.Assign(ctx, "role", familyID, "UNASSIGNED", "remain review only"); err != nil {
		t.Fatal(err)
	}
	roleHistory, err := store.Assignments(ctx, "role", familyID, 0)
	if err != nil || len(roleHistory) != 2 || roleHistory[0].TierID != "UNASSIGNED" || roleHistory[1].TierID != "UNASSIGNED" {
		t.Fatalf("role assignment history: %+v %v", roleHistory, err)
	}

	current, err := store.SaveTier(ctx, Tier{ID: tierID, Axis: "company", Name: "Test tier", Enabled: true, Priority: 4, Settings: []byte(`{"poll_minutes":30}`), Version: created.Version})
	if err != nil || current.Version != 2 {
		t.Fatalf("update tier: %+v %v", current, err)
	}
	versions, err := store.TierVersions(ctx, "company", tierID)
	if err != nil || len(versions) != 2 || versions[0].Priority != 3 || versions[1].Priority != 4 {
		t.Fatalf("tier versions: %+v %v", versions, err)
	}
	if _, err := store.SaveTier(ctx, created); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}

	// Two callers using the same version must not both overwrite the tier.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for n := 0; n < 2; n++ {
		wg.Add(1)
		go func(priority int) {
			defer wg.Done()
			_, err := store.SaveTier(ctx, Tier{ID: tierID, Axis: "company", Name: "Test tier", Enabled: true, Priority: priority, Settings: []byte(`{}`), Version: current.Version})
			results <- err
		}(n + 5)
	}
	wg.Wait()
	close(results)
	var successes, conflicts int
	for err := range results {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("concurrent edit: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent results: successes=%d conflicts=%d", successes, conflicts)
	}

	latest, err := store.ListTiers(ctx, "company")
	if err != nil {
		t.Fatal(err)
	}
	var latestTier Tier
	for _, item := range latest {
		if item.ID == tierID {
			latestTier = item
		}
	}
	disabled, err := store.SaveTier(ctx, Tier{ID: tierID, Axis: "company", Name: "Test tier", Enabled: false, Priority: latestTier.Priority, Settings: []byte(`{}`), Version: latestTier.Version})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Assign(ctx, "company", companyID, tierID, "verify disabled tier"); err != nil {
		t.Fatal(err)
	}
	_, decision, mode, err = policy.ResolveIntake(ctx, db, companyID, "validate_board", true)
	if err != nil || decision.Mode != policy.Off || mode != policy.Off {
		t.Fatalf("disabled tier allowed intake: decision=%+v mode=%s err=%v", decision, mode, err)
	}
	if disabled.Version <= current.Version {
		t.Fatalf("tier version did not advance: %+v", disabled)
	}
}
