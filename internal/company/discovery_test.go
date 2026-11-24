package company

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"karun.tech/jobagent/internal/policy"
)

func TestCandidates(t *testing.T) {
	page := `<a href="https://boards.greenhouse.io/acme">one</a>
	<a href="https://boards.greenhouse.io/acme/jobs/123">duplicate</a>
	<a href="https://jobs.ashbyhq.com/other">two</a>
	<a href="https://boards.greenhouse.io/embed/job_board?for=embedco">embed</a>
	<a href="https://boards.greenhouse.io.attacker.example/evil">bad</a>
	<script>const board = "https://jobs.ashbyhq.com/scriptco";</script>`
	found, script, err := candidates([]byte(page), "https://careers.example/")
	if err != nil || !script || len(found) != 4 {
		t.Fatalf("candidates: %+v, script=%v, err=%v", found, script, err)
	}
	expected := map[Candidate]bool{{"greenhouse", "acme"}: true, {"ashby", "other"}: true, {"greenhouse", "embedco"}: true, {"ashby", "scriptco"}: true}
	for _, candidate := range found {
		if !expected[candidate] {
			t.Fatalf("unexpected candidate: %+v", candidate)
		}
	}
}

func TestDiscoveryIntegration(t *testing.T) {
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
	version := time.Now().UnixNano()
	if _, err := db.ExecContext(ctx, "INSERT INTO policy_versions(version,global_enabled) VALUES($1,true)", version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", databaseURL)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Close()
		if _, err := cleanup.ExecContext(context.Background(), "DELETE FROM policy_versions WHERE version=$1", version); err != nil {
			t.Error(err)
		}
	})
	suffix := fmt.Sprintf("%d", version)
	host := "careers-" + suffix + ".example"
	store := Store{DB: db}
	company, err := store.CreateCompany(ctx, Company{Name: "Discovery fixture", Domain: "company-" + suffix + ".example"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateCompany(ctx, Company{Name: "Other fixture", Domain: "other-" + suffix + ".example"})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := store.CreateSeed(ctx, Seed{CompanyID: company.ID, URL: "https://" + host + "/careers?secret=private"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, e := sql.Open("pgx", databaseURL)
		if e != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM browser_exceptions WHERE seed_id=$1", seed.ID)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM ats_boards WHERE company_id IN ($1,$2)", company.ID, other.ID)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM company_seeds WHERE company_id IN ($1,$2)", company.ID, other.ID)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM source_policies WHERE source=$1", host)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE id IN ($1,$2)", company.ID, other.ID)
	})
	if _, err := db.ExecContext(ctx, "INSERT INTO source_policies (source,discovery_allowed) VALUES ($1,true)", host); err != nil {
		t.Fatal(err)
	}
	conflictKey := "conflict-" + suffix
	if _, err := store.CreateBoard(ctx, Board{CompanyID: other.ID, Provider: "greenhouse", Key: conflictKey}); err != nil {
		t.Fatal(err)
	}
	html := fmt.Sprintf(`<a href="https://boards.greenhouse.io/%s">conflict</a><a href="https://jobs.ashbyhq.com/found-%s">found</a>`, conflictKey, suffix)
	calls := 0
	inspector := NewInspector(db)
	inspector.fetch = func(ctx context.Context, target string, hosts []string, authorize func(context.Context, string) error) (int, []byte, string, error) {
		calls++
		if err := authorize(ctx, host); err != nil {
			return 0, nil, "", err
		}
		return http.StatusOK, []byte(html), target, nil
	}
	for count := 0; count < 2; count++ {
		found, err := inspector.Discover(ctx, seed.ID)
		if err != nil || len(found) != 2 {
			t.Fatalf("discover: %+v, %v", found, err)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM ats_boards WHERE company_id=$1", company.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("board count %d: %v", count, err)
	}
	var provenance string
	if err := db.QueryRowContext(ctx, "SELECT provenance::text FROM ats_boards WHERE company_id=$1", company.ID).Scan(&provenance); err != nil || strings.Contains(provenance, "secret") {
		t.Fatalf("provenance: %q %v", provenance, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM browser_exceptions WHERE seed_id=$1 AND reason='board_identity_conflict'", seed.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("conflict exceptions %d: %v", count, err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE source_policies SET discovery_allowed=false WHERE source=$1", host); err != nil {
		t.Fatal(err)
	}
	before := calls
	if _, err := inspector.Discover(ctx, seed.ID); !errors.Is(err, policy.ErrCapabilityDenied) || calls != before {
		t.Fatalf("denied fetch: %v, calls=%d", err, calls)
	}
	if _, err := db.ExecContext(ctx, "UPDATE source_policies SET discovery_allowed=true WHERE source=$1", host); err != nil {
		t.Fatal(err)
	}
	html = "<html><script src='/app.js'></script></html>"
	for count := 0; count < 2; count++ {
		if _, err := inspector.Discover(ctx, seed.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM browser_exceptions WHERE seed_id=$1 AND reason='dynamic_review'", seed.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("dynamic exceptions %d: %v", count, err)
	}
}
