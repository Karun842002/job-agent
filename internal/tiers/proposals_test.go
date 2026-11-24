package tiers

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestListProposalsIntegration(t *testing.T) {
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
	var company, proposal int64
	domain := fmt.Sprintf("proposal-list-%d.example", time.Now().UnixNano())
	if err := db.QueryRowContext(ctx, `INSERT INTO companies(name,domain) VALUES('fixture',$1) RETURNING id`, domain).Scan(&company); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM tier_proposals WHERE company_id=$1", company); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM companies WHERE id=$1", company); err != nil {
			t.Error(err)
		}
	}()
	if err := db.QueryRowContext(ctx, `INSERT INTO tier_proposals(company_id,evidence,source_url,explanation,expires_at)
		VALUES($1,'{}','https://example.com','fixture',now()+interval '1 hour') RETURNING id`, company).Scan(&proposal); err != nil {
		t.Fatal(err)
	}
	items, err := (Store{DB: db}).ListProposals(ctx, proposal-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].ID != proposal || items[0].CompanyID != company || items[0].Status != "PENDING" {
		t.Fatalf("unexpected proposals: %+v", items)
	}
	items, err = (Store{DB: db}).ListProposals(ctx, proposal)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.ID == proposal {
			t.Fatal("pagination repeated proposal")
		}
	}
	if _, err := (Store{DB: db}).ListProposals(ctx, -1); err != ErrInvalid {
		t.Fatalf("negative cursor: %v", err)
	}
}
