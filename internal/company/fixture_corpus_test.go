package company

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type scenarioBoard struct {
	Provider       string `json:"provider"`
	Key            string `json:"key"`
	State          string `json:"state"`
	Validate       bool   `json:"validate"`
	ResponseStatus int    `json:"response_status"`
}

type registryScenario struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	PageHTML           string          `json:"page_html"`
	ExpectedCandidates int             `json:"expected_candidates"`
	ExpectedBoards     int             `json:"expected_boards"`
	ExistingBoards     []scenarioBoard `json:"existing_boards"`
}

func TestRegistryFixtureCorpus(t *testing.T) {
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
	for _, step := range []string{"discover_company", "validate_board"} {
		if _, err := db.ExecContext(ctx, "INSERT INTO step_defaults(policy_version,step_key,mode) VALUES($1,$2,'LIVE')", version, step); err != nil {
			t.Fatal(err)
		}
	}
	dataDir := filepath.Join("..", "..", "testdata", "board_scenarios")
	paths, err := filepath.Glob(filepath.Join(dataDir, "*.json"))
	if err != nil || len(paths) != 4 {
		t.Fatalf("fixture paths=%v err=%v", paths, err)
	}
	var scenarios []registryScenario
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var value registryScenario
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		scenarios = append(scenarios, value)
	}
	suffix := fmt.Sprintf("%d", version)
	store := Store{DB: db}
	var companies []Company
	insertedSources := make(map[string]bool)
	insertSource := func(source string) error {
		if insertedSources[source] {
			return nil
		}
		result, err := db.ExecContext(ctx, "INSERT INTO source_policies(source,discovery_allowed) VALUES($1,true) ON CONFLICT DO NOTHING", source)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("test database already contains source policy %q", source)
		}
		insertedSources[source] = true
		return nil
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", databaseURL)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Close()
		for _, value := range companies {
			_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM ats_boards WHERE company_id=$1", value.ID)
			_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM company_seeds WHERE company_id=$1", value.ID)
			if _, err := cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE id=$1", value.ID); err != nil {
				t.Error(err)
			}
		}
		for source := range insertedSources {
			if _, err := cleanup.ExecContext(context.Background(), "DELETE FROM source_policies WHERE source=$1", source); err != nil {
				t.Error(err)
			}
		}
	})
	for _, source := range []string{"boards-api.greenhouse.io", "api.ashbyhq.com"} {
		if err := insertSource(source); err != nil {
			t.Fatal(err)
		}
	}

	for _, fixture := range scenarios {
		fixture := fixture
		t.Run(fixture.ID, func(t *testing.T) {
			host := fixture.ID + "-" + suffix + ".example"
			domain := fixture.ID + "-" + suffix + ".example"
			value, err := store.CreateCompany(ctx, Company{Name: fixture.Name, Domain: domain})
			if err != nil {
				t.Fatal(err)
			}
			companies = append(companies, value)
			seed, err := store.CreateSeed(ctx, Seed{CompanyID: value.ID, URL: "https://" + host + "/careers"})
			if err != nil {
				t.Fatal(err)
			}
			if err := insertSource(host); err != nil {
				t.Fatal(err)
			}
			for _, board := range fixture.ExistingBoards {
				key := strings.ReplaceAll(board.Key, "{{suffix}}", suffix)
				created, err := store.CreateBoard(ctx, Board{CompanyID: value.ID, Provider: board.Provider, Key: key})
				if err != nil {
					t.Fatal(err)
				}
				if board.State != "" && board.State != "candidate" {
					if err := store.SetBoardMetadata(ctx, created.ID, BoardMetadata{State: board.State, Capabilities: []byte(`{}`), Provenance: []byte(`{}`), OperatorOverride: []byte(`{}`)}); err != nil {
						t.Fatal(err)
					}
				}
			}
			page := strings.ReplaceAll(fixture.PageHTML, "{{suffix}}", suffix)
			inspector := NewInspector(db)
			inspector.fetch = func(ctx context.Context, target string, _ []string, authorize func(context.Context, string) error) (int, []byte, string, error) {
				if err := authorize(ctx, host); err != nil {
					return 0, nil, "", err
				}
				return http.StatusOK, []byte(page), target, nil
			}
			found, err := inspector.Discover(ctx, seed.ID)
			if err != nil || len(found) != fixture.ExpectedCandidates {
				t.Fatalf("first discovery=%+v err=%v", found, err)
			}
			if _, err := inspector.Discover(ctx, seed.ID); err != nil {
				t.Fatalf("repeat discovery: %v", err)
			}
			boards, err := store.ListBoards(ctx, 0)
			if err != nil {
				t.Fatal(err)
			}
			var companyBoards []Board
			for _, board := range boards {
				if board.CompanyID == value.ID {
					companyBoards = append(companyBoards, board)
				}
			}
			if len(companyBoards) != fixture.ExpectedBoards {
				t.Fatalf("persisted board count=%d want=%d", len(companyBoards), fixture.ExpectedBoards)
			}

			validator := NewValidator(db)
			for _, board := range companyBoards {
				shouldValidate := true
				responseStatus := http.StatusOK
				for _, existing := range fixture.ExistingBoards {
					if board.Key == strings.ReplaceAll(existing.Key, "{{suffix}}", suffix) {
						shouldValidate = existing.Validate
						if existing.ResponseStatus != 0 {
							responseStatus = existing.ResponseStatus
						}
					}
				}
				if !shouldValidate || board.Metadata.State == "inactive" {
					continue
				}
				for _, source := range []string{"board:" + board.Provider + ":" + board.Key} {
					if err := insertSource(source); err != nil {
						t.Fatal(err)
					}
				}
				providerHost := map[string]string{"greenhouse": "boards-api.greenhouse.io", "ashby": "api.ashbyhq.com"}[board.Provider]
				validator.fetch = func(ctx context.Context, _ string, hosts []string, authorize func(context.Context, string) error) (int, []byte, error) {
					if err := authorize(ctx, hosts[0]); err != nil {
						return 0, nil, err
					}
					if responseStatus != http.StatusOK {
						return responseStatus, nil, nil
					}
					if providerHost == "boards-api.greenhouse.io" {
						return http.StatusOK, []byte(`{"name":"Fixture"}`), nil
					}
					return http.StatusOK, []byte(`{"jobs":[{"isListed":true},{"isListed":false},{}]}`), nil
				}
				validation, err := validator.Validate(ctx, board.ID)
				if responseStatus == http.StatusForbidden {
					if err == nil {
						t.Fatal("inaccessible board validation succeeded")
					}
					persisted, getErr := store.GetBoard(ctx, board.ID)
					if getErr != nil || persisted.Metadata.State != "blocked_by_policy" {
						t.Fatalf("blocked state=%s err=%v", persisted.Metadata.State, getErr)
					}
				} else if err != nil || validation.Provider != board.Provider {
					t.Fatalf("validate %s: %+v %v", board.Key, validation, err)
				}
			}
			if fixture.ID == "migrated-ats" {
				for _, board := range companyBoards {
					if strings.HasPrefix(board.Key, "legacy-") {
						persisted, err := store.GetBoard(ctx, board.ID)
						if err != nil || persisted.Metadata.State != "inactive" {
							t.Fatalf("legacy board state=%+v err=%v", persisted.Metadata, err)
						}
					}
				}
			}
		})
	}
}
