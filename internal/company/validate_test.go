package company

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"karun.tech/jobagent/internal/policy"
)

func TestProviderValidationIntegration(t *testing.T) {
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
	store := Store{DB: db}
	company, err := store.CreateCompany(ctx, Company{Name: "Validator fixture", Domain: "validator-" + suffix + ".example"})
	if err != nil {
		t.Fatal(err)
	}
	hosts := []string{"boards-api.greenhouse.io", "api.ashbyhq.com"}
	inserted := make([]string, 0, len(hosts))
	t.Cleanup(func() {
		cleanup, e := sql.Open("pgx", databaseURL)
		if e != nil {
			return
		}
		defer cleanup.Close()
		for _, host := range inserted {
			_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM source_policies WHERE source=$1", host)
		}
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM ats_boards WHERE company_id=$1", company.ID)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE id=$1", company.ID)
	})
	for _, host := range hosts {
		result, err := db.ExecContext(ctx, "INSERT INTO source_policies(source,discovery_allowed) VALUES($1,true) ON CONFLICT DO NOTHING", host)
		if err != nil {
			t.Fatal(err)
		}
		count, err := result.RowsAffected()
		if err != nil || count != 1 {
			t.Fatalf("test database already contains source %s", host)
		}
		inserted = append(inserted, host)
	}
	greenhouse, err := store.CreateBoard(ctx, Board{CompanyID: company.ID, Provider: "greenhouse", Key: "fixture-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	ashby, err := store.CreateBoard(ctx, Board{CompanyID: company.ID, Provider: "ashby", Key: "fixture-" + suffix})
	if err != nil {
		t.Fatal(err)
	}
	validator := NewValidator(db)
	status := http.StatusOK
	body := `{"name":"Fixture"}`
	var fetchErr error
	calls := 0
	validator.fetch = func(ctx context.Context, target string, hosts []string, authorize func(context.Context, string) error) (int, []byte, error) {
		calls++
		if err := authorize(ctx, hosts[0]); err != nil {
			return 0, nil, err
		}
		return status, []byte(body), fetchErr
	}
	result, err := validator.Validate(ctx, greenhouse.ID)
	if err != nil || result.Listed != nil {
		t.Fatalf("Greenhouse result: %+v, %v", result, err)
	}
	board, err := store.GetBoard(ctx, greenhouse.ID)
	if err != nil || board.Metadata.State != "verified" || board.Metadata.LastVerifiedAt == nil || strings.Contains(string(board.Metadata.Capabilities), "listed_jobs") {
		t.Fatalf("Greenhouse metadata: %+v %v", board.Metadata, err)
	}
	body = `{"jobs":[{"isListed":true},{"isListed":false},{}]}`
	result, err = validator.Validate(ctx, ashby.ID)
	if err != nil || result.Listed == nil || *result.Listed != 1 || *result.Unlisted != 1 || *result.Unknown != 1 {
		t.Fatalf("Ashby result: %+v, %v", result, err)
	}
	board, err = store.GetBoard(ctx, ashby.ID)
	if err != nil || board.Metadata.State != "verified" || !strings.Contains(string(board.Metadata.Capabilities), "\"unlisted_jobs\": 1") {
		t.Fatalf("Ashby metadata: %+v %v", board.Metadata, err)
	}
	for _, tc := range []struct {
		status int
		body   string
		err    error
		state  string
	}{
		{http.StatusNotFound, "", nil, "degraded"},
		{http.StatusForbidden, "", nil, "blocked_by_policy"},
		{http.StatusTooManyRequests, "", nil, "degraded"},
		{http.StatusOK, `{"bad":"schema"}`, nil, "degraded"},
		{0, "", context.DeadlineExceeded, "degraded"},
	} {
		status, body, fetchErr = tc.status, tc.body, tc.err
		if _, err := validator.Validate(ctx, ashby.ID); err == nil {
			t.Fatal("failed validation succeeded")
		}
		current, err := store.GetBoard(ctx, ashby.ID)
		if err != nil || current.Metadata.State != tc.state || current.Metadata.LastVerifiedAt == nil {
			t.Fatalf("state: %+v %v", current.Metadata, err)
		}
	}
	if _, err := db.ExecContext(ctx, "UPDATE source_policies SET discovery_allowed=false WHERE source=$1", hosts[0]); err != nil {
		t.Fatal(err)
	}
	before := calls
	if _, err := validator.Validate(ctx, greenhouse.ID); !errors.Is(err, policy.ErrCapabilityDenied) || calls != before {
		t.Fatalf("policy denial: %v", err)
	}
	board, err = store.GetBoard(ctx, greenhouse.ID)
	if err != nil || board.Metadata.State != "blocked_by_policy" {
		t.Fatalf("blocked state: %+v %v", board.Metadata, err)
	}
	handler := Handler(store, "fixture-token")
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/boards/%d/validate", greenhouse.ID), nil)
	request.Header.Set("Authorization", "Bearer fixture-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("API policy denial = %d", response.Code)
	}
	if _, err := db.ExecContext(ctx, "UPDATE source_policies SET discovery_allowed=true WHERE source=$1", hosts[0]); err != nil {
		t.Fatal(err)
	}
	validator.fetch = func(context.Context, string, []string, func(context.Context, string) error) (int, []byte, error) {
		_, err := db.ExecContext(ctx, "UPDATE ats_boards SET board_key=board_key || '-changed' WHERE id=$1", greenhouse.ID)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusOK, []byte(`{"name":"Fixture"}`), nil
	}
	if _, err := validator.Validate(ctx, greenhouse.ID); err == nil || !strings.Contains(err.Error(), "changed during validation") {
		t.Fatalf("stale board validation was accepted: %v", err)
	}
}
