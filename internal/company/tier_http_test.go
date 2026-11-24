package company

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestTierRoutesRequireBearerAuthentication(t *testing.T) {
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
	store := Store{DB: db}
	suffix := strings.ToUpper(fmt.Sprintf("%x", time.Now().UnixNano()))
	company, err := store.CreateCompany(ctx, Company{Name: "Tier API fixture", Domain: "tier-api-" + strings.ToLower(suffix) + ".example"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(context.Background(), "DELETE FROM companies WHERE id=$1", company.ID) })
	handler := Handler(store, "integration-token")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/tiers/company", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated tier read = %d", response.Code)
	}

	tierID := "API_" + suffix
	request = httptest.NewRequest(http.MethodPut, "/api/v1/tiers/companies", bytes.NewBufferString(fmt.Sprintf(`{"id":%q,"name":"API fixture","enabled":true,"priority":1,"settings":{}}`, tierID)))
	request.Header.Set("Authorization", "Bearer integration-token")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("create tier = %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/companies/%d/tier", company.ID), bytes.NewBufferString(fmt.Sprintf(`{"tier_id":%q,"reason":"integration API"}`, tierID)))
	request.Header.Set("Authorization", "Bearer integration-token")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("assign tier = %d: %s", response.Code, response.Body.String())
	}
	updated, err := store.GetCompany(ctx, company.ID)
	if err != nil || updated.TierID != tierID {
		t.Fatalf("company assignment %+v: %v", updated, err)
	}
}
