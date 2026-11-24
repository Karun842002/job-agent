package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"karun.tech/jobagent/internal/company"
)

func TestRegistryAdminCommandsIntegration(t *testing.T) {
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	domain := "admin-fixture-" + suffix + ".example"
	store := company.Store{DB: db}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", databaseURL)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(context.Background(), `DELETE FROM company_seeds WHERE company_id IN
			(SELECT id FROM companies WHERE domain=$1)`, domain)
		_, _ = cleanup.ExecContext(context.Background(), `DELETE FROM ats_boards WHERE company_id IN
			(SELECT id FROM companies WHERE domain=$1)`, domain)
		if _, err := cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE domain=$1", domain); err != nil {
			t.Error(err)
		}
	})
	filePath := filepath.Join(t.TempDir(), "companies.csv")
	csvData := "name,domain,careers_url\nFixture, " + domain + ",https://careers." + domain + "/jobs\n"
	if err := os.WriteFile(filePath, []byte(csvData), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cli := app{db: db, out: &output}
	if err := cli.run(ctx, []string{"companies", "import", "--format", "csv", "--file", filePath}); err != nil {
		t.Fatalf("companies import: %v", err)
	}
	if !strings.Contains(output.String(), `"imported":1`) {
		t.Fatalf("import output: %s", output.String())
	}
	var companyID, seedID int64
	if err := db.QueryRowContext(ctx, "SELECT c.id,s.id FROM companies c JOIN company_seeds s ON s.company_id=c.id WHERE c.domain=$1", domain).Scan(&companyID, &seedID); err != nil {
		t.Fatal(err)
	}
	boardKey := "admin-" + suffix
	board, err := store.CreateBoard(ctx, company.Board{CompanyID: companyID, Provider: "greenhouse", Key: boardKey})
	if err != nil {
		t.Fatal(err)
	}

	output.Reset()
	if err := cli.run(ctx, []string{"boards", "list", "--after", fmt.Sprint(board.ID - 1)}); err != nil {
		t.Fatalf("boards list: %v", err)
	}
	if !strings.Contains(output.String(), `"board_key":"`+boardKey+`"`) {
		t.Fatalf("list output: %s", output.String())
	}

	discoveryCalls := 0
	cli.discover = func(_ context.Context, id int64) ([]company.Candidate, error) {
		discoveryCalls++
		if id != seedID {
			t.Fatalf("discovery seed=%d, want %d", id, seedID)
		}
		candidate := company.Candidate{Provider: "ashby", Key: "discovered-" + suffix}
		_, err := store.CreateBoard(ctx, company.Board{CompanyID: companyID, Provider: candidate.Provider, Key: candidate.Key})
		if err != nil {
			return nil, err
		}
		return []company.Candidate{candidate}, nil
	}
	output.Reset()
	if err := cli.run(ctx, []string{"boards", "discover", "--seed", fmt.Sprint(seedID)}); err != nil {
		t.Fatalf("boards discover: %v", err)
	}
	if !strings.Contains(output.String(), `"provider":"ashby"`) || discoveryCalls != 1 {
		t.Fatalf("discover output=%s calls=%d", output.String(), discoveryCalls)
	}
	if err := cli.run(ctx, []string{"boards", "discover", "--seed", "not-an-id"}); err == nil || discoveryCalls != 1 {
		t.Fatalf("invalid discovery ID: err=%v calls=%d", err, discoveryCalls)
	}
	var persisted int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM ats_boards WHERE company_id=$1 AND board_key=$2", companyID, "discovered-"+suffix).Scan(&persisted); err != nil || persisted != 1 {
		t.Fatalf("discovery persistence=%d err=%v", persisted, err)
	}

	validateCalls := 0
	cli.validate = func(_ context.Context, id int64) (company.ValidationResult, error) {
		validateCalls++
		if id != board.ID {
			t.Fatalf("validation board=%d, want %d", id, board.ID)
		}
		return company.ValidationResult{Provider: "greenhouse"}, nil
	}
	output.Reset()
	if err := cli.run(ctx, []string{"boards", "validate", "--id", fmt.Sprint(board.ID)}); err != nil {
		t.Fatalf("boards validate: %v", err)
	}
	if !strings.Contains(output.String(), `"provider":"greenhouse"`) || validateCalls != 1 {
		t.Fatalf("validate output=%s calls=%d", output.String(), validateCalls)
	}
	if err := cli.run(ctx, []string{"boards", "validate", "--id", "0"}); err == nil || validateCalls != 1 {
		t.Fatalf("invalid validation ID: err=%v calls=%d", err, validateCalls)
	}

	if err := cli.run(ctx, []string{"companies", "import", "--format", "csv", "--file", filePath}); err == nil {
		t.Fatal("duplicate import succeeded")
	}
	var importedCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM companies WHERE domain=$1", domain).Scan(&importedCount); err != nil || importedCount != 1 {
		t.Fatalf("duplicate import persisted %d companies: %v", importedCount, err)
	}
}

func TestAdminRejectsUnknownAndMalformedCommands(t *testing.T) {
	for _, args := range [][]string{{}, {"boards", "remove"}, {"boards", "list", "--after", "-1"}, {"companies", "import", "--format", "xml", "--file", "unused"}} {
		if err := (app{}).run(context.Background(), args); err == nil {
			t.Fatalf("accepted command %q", args)
		}
	}
}
