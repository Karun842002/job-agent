package policy

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestResolveStep(t *testing.T) {
	base := StepInput{
		StepKey: "prepare_resume", PolicyVersion: 1, GlobalEnabled: true, SourceAllowed: true,
		Company: Axis{TierID: "T1", Enabled: true, Mode: Live},
		Role:    Axis{TierID: "CORE", Enabled: true, Mode: Live}, Default: Audit,
	}
	tests := []struct {
		name   string
		change func(*StepInput)
		want   Mode
	}{
		{"live", func(*StepInput) {}, Live},
		{"axis default", func(i *StepInput) { i.Company.Mode = "" }, Audit},
		{"audit wins", func(i *StepInput) { i.Role.Mode = Audit }, Audit},
		{"off wins", func(i *StepInput) { i.Company.Mode = Off }, Off},
		{"disabled company", func(i *StepInput) { i.Company.Enabled = false }, Off},
		{"disabled role", func(i *StepInput) { i.Role.Enabled = false }, Off},
		{"emergency", func(i *StepInput) { i.GlobalEnabled = false }, Off},
		{"denied source", func(i *StepInput) { i.SourceAllowed = false }, Off},
		{"unassigned", func(i *StepInput) { i.Company.TierID = "UNASSIGNED" }, Off},
		{"excluded", func(i *StepInput) { i.Role.TierID = "EXCLUDED" }, Off},
		{"unassigned intake", func(i *StepInput) { i.Company.TierID = "UNASSIGNED"; i.StepKey = "tier_proposal" }, Live},
		{"hard limit", func(i *StepInput) { i.HardLimit = Audit }, Audit},
		{"override cannot elevate", func(i *StepInput) { i.Company.Mode = Audit; i.JobOverride = Live }, Audit},
		{"submission needs approval", func(i *StepInput) { i.StepKey = "submit_application" }, Off},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.change(&input)
			got, err := ResolveStep(input)
			if err != nil || got.Mode != tc.want || got.Reason == "" {
				t.Fatalf("got %+v, %v, want %s with reason", got, err, tc.want)
			}
		})
	}
	base.Company.Mode = "UNKNOWN"
	if got, err := ResolveStep(base); err == nil || got.Mode != Off {
		t.Fatalf("invalid mode must fail closed: %+v, %v", got, err)
	}
}

func TestRecordStepIntegration(t *testing.T) {
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
	if _, err := tx.ExecContext(ctx, "INSERT INTO policy_versions (version, global_enabled) VALUES (5, true)"); err != nil {
		t.Fatal(err)
	}
	input := StepInput{StepKey: "prepare_resume", PolicyVersion: 5, GlobalEnabled: true, SourceAllowed: true,
		Company: Axis{TierID: "T1", Enabled: true}, Role: Axis{TierID: "CORE", Enabled: true}, Default: Audit}
	id, decision, err := RecordStep(ctx, tx, input)
	if err != nil || decision.Mode != Audit {
		t.Fatalf("record: %+v, %v", decision, err)
	}
	var version int64
	var mode string
	if err := tx.QueryRowContext(ctx, "SELECT policy_version, mode FROM effective_step_policies WHERE id = $1", id).Scan(&version, &mode); err != nil {
		t.Fatal(err)
	}
	if version != 5 || mode != "AUDIT" {
		t.Fatalf("snapshot = %d, %s", version, mode)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE effective_step_policies SET mode = 'LIVE' WHERE id = $1", id); err == nil {
		t.Fatal("snapshot update was accepted")
	}
}
