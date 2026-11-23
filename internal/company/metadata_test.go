package company

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"karun.tech/jobagent/internal/policy"
)

func TestBoardMetadataIntegration(t *testing.T) {
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
	suffix := time.Now().UnixNano()
	store := Store{DB: db}
	company, err := store.CreateCompany(ctx, Company{Name: "Metadata fixture", Domain: fmt.Sprintf("meta-%d.example", suffix)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", url)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM source_policies WHERE source=$1", company.Domain)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM ats_boards WHERE company_id=$1", company.ID)
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE id=$1", company.ID)
	})
	board, err := store.CreateBoard(ctx, Board{CompanyID: company.ID, Provider: "ashby", Key: fmt.Sprintf("fixture-%d", suffix)})
	if err != nil {
		t.Fatal(err)
	}
	if board.Metadata.State != "candidate" {
		t.Fatalf("default state %q", board.Metadata.State)
	}
	verified := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := db.ExecContext(ctx, "INSERT INTO source_policies (source) VALUES ($1)", company.Domain); err != nil {
		t.Fatal(err)
	}
	value := BoardMetadata{State: "verified", Capabilities: json.RawMessage(`{"public_read":true}`),
		Provenance: json.RawMessage(`{"source":"fixture"}`), LastVerifiedAt: &verified,
		OperatorOverride: json.RawMessage(`{"note":"manual","discovery_allowed":true}`)}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	handler := Handler(store, "fixture-token")
	path := fmt.Sprintf("/api/v1/boards/%d/metadata", board.ID)
	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body)))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized = %d", unauthorized.Code)
	}
	request := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer fixture-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("update = %d: %s", response.Code, response.Body)
	}
	otherDB, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	reloaded, err := (Store{DB: otherDB}).GetBoard(ctx, board.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Metadata.State != value.State || reloaded.Metadata.LastVerifiedAt == nil || !reloaded.Metadata.LastVerifiedAt.Equal(verified) ||
		!jsonEqual(reloaded.Metadata.Capabilities, value.Capabilities) ||
		!jsonEqual(reloaded.Metadata.Provenance, value.Provenance) ||
		!jsonEqual(reloaded.Metadata.OperatorOverride, value.OperatorOverride) {
		t.Fatalf("metadata after reconnect: %+v", reloaded.Metadata)
	}
	value.State = "invented"
	if err := store.SetBoardMetadata(ctx, board.ID, value); err == nil {
		t.Fatal("invalid state accepted")
	}
	value.State = "verified"
	value.Capabilities = json.RawMessage(`[]`)
	if err := store.SetBoardMetadata(ctx, board.ID, value); err == nil {
		t.Fatal("invalid capabilities accepted")
	}
	if err := policy.CheckSource(ctx, db, company.Domain, policy.Discovery); !errors.Is(err, policy.ErrCapabilityDenied) {
		t.Fatalf("override changed source permission: %v", err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE company_id=$1 AND event_name='board_metadata_updated'", company.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("metadata audit count %d: %v", count, err)
	}
}

func jsonEqual(left, right json.RawMessage) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
