package audit

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestPreviewIsolationIntegration(t *testing.T) {
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
	version := time.Now().UnixNano()
	if _, err := db.ExecContext(ctx, "INSERT INTO policy_versions (version, global_enabled) VALUES ($1, true)", version); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", url)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM audit_previews WHERE policy_snapshot_id IN (SELECT id FROM effective_step_policies WHERE policy_version = $1)", version)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM effective_step_policies WHERE policy_version = $1", version)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM policy_versions WHERE version = $1", version)
	})
	var snapshot int64
	if err := db.QueryRowContext(ctx, `INSERT INTO effective_step_policies
		(step_key, policy_version, inputs, mode, reason)
		VALUES ('fill_form', $1, '{}', 'AUDIT', 'fixture') RETURNING id`, version).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM app_metadata").Scan(&before); err != nil {
		t.Fatal(err)
	}
	preview := Preview{PolicySnapshotID: snapshot, StepRunID: "test-step", Diff: []byte(`{"would_fill":"field"}`)}
	id, err := SavePreview(ctx, db, preview)
	if err != nil || id == 0 {
		t.Fatalf("preview id %d: %v", id, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM app_metadata").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("preview changed production state")
	}
	var saved bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM audit_previews WHERE id=$1)", id).Scan(&saved); err != nil || !saved {
		t.Fatalf("preview persistence: %v", err)
	}
	var liveSnapshot int64
	if err := db.QueryRowContext(ctx, `INSERT INTO effective_step_policies
		(step_key, policy_version, inputs, mode, reason)
		VALUES ('fill_form', $1, '{}', 'LIVE', 'fixture') RETURNING id`, version).Scan(&liveSnapshot); err != nil {
		t.Fatal(err)
	}
	preview.PolicySnapshotID = liveSnapshot
	if _, err := SavePreview(ctx, db, preview); err == nil {
		t.Fatal("non-AUDIT policy accepted")
	}
	preview.PolicySnapshotID = 0
	if _, err := SavePreview(ctx, db, preview); err == nil {
		t.Fatal("missing policy accepted")
	}
	preview.PolicySnapshotID = snapshot
	preview.Diff = []byte("invalid")
	if _, err := SavePreview(ctx, db, preview); err == nil {
		t.Fatal("invalid diff accepted")
	}
}
