package tiers

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestProposalIdentityConstraintsIntegration(t *testing.T) {
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
	var a, b, role int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO companies(name,domain) VALUES('fixture','proposal-a.example') RETURNING id`).Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO companies(name,domain) VALUES('fixture','proposal-b.example') RETURNING id`).Scan(&b); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRowContext(ctx, `INSERT INTO role_families(name) VALUES('fixture') RETURNING id`).Scan(&role); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO tier_proposals(company_id,role_family_id,evidence,source_url,explanation,expires_at)
		VALUES($1,$2,'{}','https://example.com','fixture',now()+interval '1 hour')`
	if _, err := tx.ExecContext(ctx, insert, a, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, insert, a, role); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []struct {
		company int64
		role    any
	}{{a, nil}, {b, role}} {
		if _, err := tx.ExecContext(ctx, "SAVEPOINT duplicate"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, insert, candidate.company, candidate.role); err == nil {
			t.Fatal("duplicate company or role proposal was accepted")
		}
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT duplicate"); err != nil {
			t.Fatal(err)
		}
	}
}
