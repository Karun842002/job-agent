package company

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"karun.tech/jobagent/internal/browser"
)

type dynamicPageHarness struct {
	server     *httptest.Server
	rendered   string
	candidates []Candidate
}

func (h *dynamicPageHarness) NotifyQueued(ctx context.Context, _ browser.BrowserTaskID) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, h.server.URL+"/careers", nil)
	if err != nil {
		return err
	}
	response, err := h.server.Client().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "dynamic-content-pending") || !strings.Contains(string(body), "<script") {
		return fmt.Errorf("dynamic fixture was not served: status=%d", response.StatusCode)
	}
	// The mock harness supplies the rendered DOM snapshot that a permitted READ
	// task would return after the page's script populates the job-board link.
	found, _, err := candidates([]byte(h.rendered), h.server.URL+"/careers")
	if err != nil {
		return err
	}
	h.candidates = found
	return nil
}

func TestMockHarnessDiscoversAndValidatesDynamicBoard(t *testing.T) {
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
	var snapshotID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO effective_step_policies(step_key,policy_version,inputs,mode,reason)
		VALUES('discover_company',$1,'{}','LIVE','mock browser test') RETURNING id`, version).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", version)
	source := "dynamic-careers-" + suffix + ".example"
	workflowID := "dynamic-fixture-" + suffix
	company, err := (Store{DB: db}).CreateCompany(ctx, Company{Name: "Dynamic page fixture", Domain: "dynamic-company-" + suffix + ".example"})
	if err != nil {
		t.Fatal(err)
	}
	page := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/careers" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><body><div id="jobs">dynamic-content-pending</div><script>loadJobs()</script></body></html>`)
	}))
	defer page.Close()
	origin := strings.TrimSuffix(page.URL, "/")
	boardScope := "board:ashby:dynamic-" + suffix
	insertedSources := make([]string, 0, 3)
	for _, value := range []string{source, "api.ashbyhq.com", boardScope} {
		result, err := db.ExecContext(ctx, "INSERT INTO source_policies(source,discovery_allowed) VALUES($1,true) ON CONFLICT DO NOTHING", value)
		if err != nil {
			t.Fatal(err)
		}
		count, err := result.RowsAffected()
		if err != nil || count != 1 {
			t.Fatalf("test source policy already exists: %s", value)
		}
		insertedSources = append(insertedSources, value)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO source_browser_origins(source,origin) VALUES($1,$2)", source, origin); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", databaseURL)
		if err != nil {
			t.Error(err)
			return
		}
		defer cleanup.Close()
		if _, err := cleanup.ExecContext(context.Background(), "DELETE FROM browser_tasks WHERE workflow_id=$1", workflowID); err != nil {
			t.Error(err)
		}
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM source_browser_origins WHERE source=$1", source)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM ats_boards WHERE company_id=$1", company.ID)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE id=$1", company.ID)
		for _, value := range insertedSources {
			if _, err := cleanup.ExecContext(context.Background(), "DELETE FROM source_policies WHERE source=$1", value); err != nil {
				t.Error(err)
			}
		}
	})
	mock := &dynamicPageHarness{server: page, rendered: `<html><body><a href="https://jobs.ashbyhq.com/dynamic-` + suffix + `">Current openings</a></body></html>`}
	task := browser.BrowserTask{WorkflowID: workflowID, StepRunID: "dynamic-read", Source: source,
		Origin: origin, AllowedOrigins: []string{origin}, Action: "NAVIGATE", AllowedActions: []string{"NAVIGATE", "READ"},
		PolicySnapshotID: snapshotID, PolicyVersion: version, Deadline: time.Now().Add(time.Minute), CallbackID: "dynamic-" + suffix}
	broker := browser.Broker{DB: db}
	id, err := (browser.HarnessAdapter{Broker: broker, Transport: mock}).Dispatch(ctx, task)
	if err != nil {
		t.Fatalf("mock harness dispatch: %v", err)
	}
	if status, err := broker.Status(ctx, id); err != nil || status != browser.Queued {
		t.Fatalf("queued status=%s err=%v", status, err)
	}
	if len(mock.candidates) != 1 || mock.candidates[0] != (Candidate{Provider: "ashby", Key: "dynamic-" + suffix}) {
		t.Fatalf("dynamic candidates: %+v", mock.candidates)
	}
	readTask := task
	readTask.Action, readTask.CallbackID = "READ", "read-"+suffix
	if _, err := broker.Dispatch(ctx, readTask); err != nil {
		t.Fatalf("permitted READ task: %v", err)
	}
	deniedOrigin := task
	deniedOrigin.Origin, deniedOrigin.AllowedOrigins, deniedOrigin.CallbackID = "https://unlisted.example", []string{"https://unlisted.example"}, "denied-origin-"+suffix
	if _, err := broker.Dispatch(ctx, deniedOrigin); err == nil {
		t.Fatal("unregistered origin was accepted")
	}
	deniedAction := task
	deniedAction.Action, deniedAction.AllowedActions, deniedAction.CallbackID = "SUBMIT", []string{"SUBMIT"}, "denied-action-"+suffix
	if _, err := broker.Dispatch(ctx, deniedAction); err == nil {
		t.Fatal("mutating browser action was accepted")
	}

	board, err := (Store{DB: db}).CreateBoard(ctx, Board{CompanyID: company.ID, Provider: mock.candidates[0].Provider, Key: mock.candidates[0].Key})
	if err != nil {
		t.Fatal(err)
	}
	validator := NewValidator(db)
	validator.fetch = func(ctx context.Context, _ string, hosts []string, authorize func(context.Context, string) error) (int, []byte, error) {
		if err := authorize(ctx, hosts[0]); err != nil {
			return 0, nil, err
		}
		return http.StatusOK, []byte(`{"jobs":[{"isListed":true}]}`), nil
	}
	result, err := validator.Validate(ctx, board.ID)
	if err != nil || result.Provider != "ashby" || result.Listed == nil || *result.Listed != 1 {
		t.Fatalf("candidate validation: %+v err=%v", result, err)
	}
	verified, err := (Store{DB: db}).GetBoard(ctx, board.ID)
	if err != nil || verified.Metadata.State != "verified" {
		t.Fatalf("candidate board state=%q err=%v", verified.Metadata.State, err)
	}
}
