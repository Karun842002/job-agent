package audit

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAppendAndProtectIntegration(t *testing.T) {
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
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	trace := "4bf92f3577b34da6a3ce929d0e0e4736"
	step := "step-1"
	evidence := "a6f55038-c5bd-4f67-8937-f049f3440592"
	id, err := Append(ctx, tx, Event{Name: "step_started", Actor: "system", Cause: "scheduled", TraceID: &trace, StepRunID: &step, EvidenceRef: &evidence})
	if err != nil {
		t.Fatal(err)
	}
	var storedTrace, storedStep, storedEvidence string
	if err := tx.QueryRowContext(ctx, "SELECT trace_id, step_run_id, evidence_ref FROM audit_events WHERE id=$1", id).Scan(&storedTrace, &storedStep, &storedEvidence); err != nil {
		t.Fatal(err)
	}
	if storedTrace != trace || storedStep != step || storedEvidence != evidence {
		t.Fatal("correlation fields were not preserved")
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT audit_update"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE audit_events SET actor='other' WHERE id=$1", id); err == nil {
		t.Fatal("audit update succeeded")
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT audit_update"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "SAVEPOINT audit_delete"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM audit_events WHERE id=$1", id); err == nil {
		t.Fatal("early audit deletion succeeded")
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT audit_delete"); err != nil {
		t.Fatal(err)
	}
	var expiredID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO audit_events (created_at, event_name, actor, cause)
		VALUES (now() - interval '25 hours', 'expired', 'system', 'test') RETURNING id`).Scan(&expiredID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM audit_events WHERE id=$1", expiredID); err != nil {
		t.Fatalf("expired event deletion: %v", err)
	}
}
