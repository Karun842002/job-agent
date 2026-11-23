package browser

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

type mockTransport struct {
	ids []BrowserTaskID
	err error
}

func (m *mockTransport) NotifyQueued(_ context.Context, id BrowserTaskID) error {
	m.ids = append(m.ids, id)
	return m.err
}

func TestStubFailsClosed(t *testing.T) {
	var runner BrowserTaskRunner = StubRunner{}
	if _, err := runner.Dispatch(context.Background(), BrowserTask{}); err == nil {
		t.Fatal("unconfigured harness accepted task")
	}
}

func TestBrokerIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	version := time.Now().UnixNano()
	source := fmt.Sprintf("browser-test-%d", version)
	origin := "https://example.org"
	var snapshot int64
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{"INSERT INTO source_policies (source, discovery_allowed) VALUES ($1, true)", []any{source}},
		{"INSERT INTO source_browser_origins (source, origin) VALUES ($1, $2)", []any{source, origin}},
		{"INSERT INTO policy_versions (version, global_enabled) VALUES ($1, true)", []any{version}},
	} {
		if _, err := db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM browser_tasks WHERE source = $1", source)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM source_browser_origins WHERE source = $1", source)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM source_policies WHERE source = $1", source)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM effective_step_policies WHERE policy_version = $1", version)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM policy_versions WHERE version = $1", version)
		_ = db.Close()
	})
	err = db.QueryRowContext(ctx, `INSERT INTO effective_step_policies
		(step_key, policy_version, inputs, mode, reason) VALUES
		('discover_company', $1, '{}', 'AUDIT', 'fixture') RETURNING id`, version).Scan(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	task := BrowserTask{WorkflowID: "workflow", StepRunID: "step", Source: source, Origin: origin,
		AllowedOrigins: []string{origin}, Action: "READ", AllowedActions: []string{"READ"},
		PolicySnapshotID: snapshot, PolicyVersion: version, Deadline: time.Now().Add(time.Minute), CallbackID: source}
	broker := Broker{DB: db}
	for name, change := range map[string]func(*BrowserTask){
		"unlisted origin":      func(t *BrowserTask) { t.Origin = "https://untrusted.example"; t.AllowedOrigins = []string{t.Origin} },
		"unapproved action":    func(t *BrowserTask) { t.Action = "SUBMIT"; t.AllowedActions = []string{"SUBMIT"} },
		"expired":              func(t *BrowserTask) { t.Deadline = time.Now().Add(-time.Second) },
		"wrong policy version": func(t *BrowserTask) { t.PolicyVersion++ },
	} {
		t.Run(name, func(t *testing.T) {
			copy := task
			change(&copy)
			if _, err := broker.Dispatch(ctx, copy); err == nil {
				t.Fatal("task was accepted")
			}
		})
	}
	transport := &mockTransport{}
	adapter := HarnessAdapter{Broker: broker, Transport: transport}
	id, err := adapter.Dispatch(ctx, task)
	if err != nil {
		t.Fatal(err)
	}
	if len(transport.ids) != 1 || transport.ids[0] != id {
		t.Fatal("transport did not receive queued task ID")
	}
	if _, err := adapter.Dispatch(ctx, task); err == nil {
		t.Fatal("duplicate callback accepted")
	}
	failedTask := task
	failedTask.CallbackID += "-failed-notification"
	transport.err = errors.New("offline")
	failedID, err := adapter.Dispatch(ctx, failedTask)
	if err == nil || failedID == 0 {
		t.Fatal("failed notification must return the durable task ID")
	}
	if state, err := broker.Status(ctx, failedID); err != nil || state != Queued {
		t.Fatalf("failed notification lost task: %q, %v", state, err)
	}
	otherDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	restarted := Broker{DB: otherDB}
	status, err := restarted.Status(ctx, id)
	if err != nil || status != Queued {
		t.Fatalf("status %q: %v", status, err)
	}
	if err := restarted.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	status, err = broker.Status(ctx, id)
	if err != nil || status != Cancelled {
		t.Fatalf("status %q: %v", status, err)
	}
}
