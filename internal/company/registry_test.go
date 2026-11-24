package company

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestRegistryIntegration(t *testing.T) {
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	domain := "registry-" + suffix + ".example"
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", url)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM ats_boards WHERE company_id IN (SELECT id FROM companies WHERE domain LIKE $1)", "%"+suffix+"%")
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM company_seeds WHERE company_id IN (SELECT id FROM companies WHERE domain LIKE $1)", "%"+suffix+"%")
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE domain LIKE $1", "%"+suffix+"%")
	})
	handler := Handler(Store{DB: db}, "fixture-token")
	request := func(method, path, body, token string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	body := fmt.Sprintf(`{"name":"Example","domain":%q}`, domain)
	for _, token := range []string{"", "wrong"} {
		if got := request("POST", "/api/v1/companies", body, token).Code; got != http.StatusUnauthorized {
			t.Fatalf("unauthorized POST = %d", got)
		}
	}
	var company Company
	created := request("POST", "/api/v1/companies", body, "fixture-token")
	if created.Code != http.StatusCreated {
		t.Fatalf("create company: %d %s", created.Code, created.Body)
	}
	if err := json.Unmarshal(created.Body.Bytes(), &company); err != nil {
		t.Fatal(err)
	}
	if got := request("POST", "/api/v1/companies", body, "fixture-token").Code; got != http.StatusConflict {
		t.Fatalf("duplicate company = %d", got)
	}
	companyPath := fmt.Sprintf("/api/v1/companies/%d", company.ID)
	if got := request("GET", companyPath, "", "fixture-token").Code; got != http.StatusOK {
		t.Fatalf("get company = %d", got)
	}
	if got := request("PUT", companyPath, body, "fixture-token").Code; got != http.StatusNoContent {
		t.Fatalf("update company = %d", got)
	}
	seedBody := fmt.Sprintf(`{"company_id":%d,"url":"https://example.com/careers"}`, company.ID)
	seedResponse := request("POST", "/api/v1/seeds", seedBody, "fixture-token")
	if seedResponse.Code != http.StatusCreated {
		t.Fatalf("create seed: %d %s", seedResponse.Code, seedResponse.Body)
	}
	var seed Seed
	if err := json.Unmarshal(seedResponse.Body.Bytes(), &seed); err != nil {
		t.Fatal(err)
	}
	seedPath := fmt.Sprintf("/api/v1/seeds/%d", seed.ID)
	if got := request("GET", seedPath, "", "fixture-token").Code; got != http.StatusOK {
		t.Fatalf("get seed = %d", got)
	}
	if got := request("PUT", seedPath, seedBody, "fixture-token").Code; got != http.StatusNoContent {
		t.Fatalf("update seed = %d", got)
	}
	var boards []Board
	for _, key := range []string{"first", "second"} {
		boardBody := fmt.Sprintf(`{"company_id":%d,"provider":"greenhouse","board_key":%q}`, company.ID, key+"-"+suffix)
		response := request("POST", "/api/v1/boards", boardBody, "fixture-token")
		if response.Code != http.StatusCreated {
			t.Fatalf("create board: %d %s", response.Code, response.Body)
		}
		var board Board
		if err := json.Unmarshal(response.Body.Bytes(), &board); err != nil {
			t.Fatal(err)
		}
		boards = append(boards, board)
		path := fmt.Sprintf("/api/v1/boards/%d", board.ID)
		if got := request("GET", path, "", "fixture-token").Code; got != http.StatusOK {
			t.Fatalf("get board = %d", got)
		}
		if got := request("PUT", path, boardBody, "fixture-token").Code; got != http.StatusNoContent {
			t.Fatalf("update board = %d", got)
		}
	}
	if got := request("DELETE", companyPath, "", "fixture-token").Code; got != http.StatusConflict {
		t.Fatalf("dependent company delete = %d", got)
	}
	list := request("GET", fmt.Sprintf("/api/v1/boards?after=%d", boards[0].ID-1), "", "fixture-token")
	var listed []Board
	if list.Code != http.StatusOK || json.Unmarshal(list.Body.Bytes(), &listed) != nil || len(listed) < 2 {
		t.Fatalf("board listing: %d %s", list.Code, list.Body)
	}
	if got := request("GET", "/api/v1/boards?after=invalid", "", "fixture-token").Code; got != http.StatusBadRequest {
		t.Fatalf("invalid cursor = %d", got)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM ats_boards WHERE company_id=$1", company.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("board count %d: %v", count, err)
	}
	for _, board := range boards {
		if got := request("DELETE", fmt.Sprintf("/api/v1/boards/%d", board.ID), "", "fixture-token").Code; got != http.StatusNoContent {
			t.Fatalf("delete board = %d", got)
		}
	}
	if got := request("DELETE", seedPath, "", "fixture-token").Code; got != http.StatusNoContent {
		t.Fatalf("delete seed = %d", got)
	}
	if got := request("DELETE", companyPath, "", "fixture-token").Code; got != http.StatusNoContent {
		t.Fatalf("delete company = %d", got)
	}
	if got := request("GET", companyPath, "", "fixture-token").Code; got != http.StatusNotFound {
		t.Fatalf("deleted company = %d", got)
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE company_id=$1", company.ID).Scan(&auditCount); err != nil || auditCount != 12 {
		t.Fatalf("company audit count %d: %v", auditCount, err)
	}
	if got := request("POST", "/api/v1/companies", `{"name":"x","domain":"x","unknown":true}`, "fixture-token").Code; got != http.StatusBadRequest {
		t.Fatalf("unknown field = %d", got)
	}
}

func TestImportIntegration(t *testing.T) {
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
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	t.Cleanup(func() {
		cleanup, err := sql.Open("pgx", url)
		if err != nil {
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM company_seeds WHERE company_id IN (SELECT id FROM companies WHERE domain LIKE $1)", "%"+suffix+"%")
		_, _ = cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE domain LIKE $1", "%"+suffix+"%")
	})
	store := Store{DB: db}
	csvData := fmt.Sprintf("name,domain,careers_url\nAlpha,a-%s.example,https://a.example/careers\n", suffix)
	if count, err := store.Import(ctx, "csv", strings.NewReader(csvData)); err != nil || count != 1 {
		t.Fatalf("CSV import %d: %v", count, err)
	}
	yamlData := fmt.Sprintf("- name: Beta\n  domain: b-%s.example\n  careers_url: https://b.example/careers\n", suffix)
	if count, err := store.Import(ctx, "yaml", strings.NewReader(yamlData)); err != nil || count != 1 {
		t.Fatalf("YAML import %d: %v", count, err)
	}
	duplicate := fmt.Sprintf("name,domain,careers_url\nNew,new-%s.example,https://new.example\nDuplicate,a-%s.example,https://a.example\n", suffix, suffix)
	if _, err := store.Import(ctx, "csv", strings.NewReader(duplicate)); err == nil {
		t.Fatal("duplicate import succeeded")
	}
	var exists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM companies WHERE domain=$1)", "new-"+suffix+".example").Scan(&exists); err != nil || exists {
		t.Fatalf("partial import: %v, %v", exists, err)
	}
	for _, tc := range []struct{ format, value string }{
		{"csv", "wrong,header\nx,y\n"},
		{"yaml", "- name: X\n  domain: x\n  unknown: y\n"},
		{"yaml", "- name: X\n  domain: x\n  careers_url: y\n---\n- name: Y\n"},
	} {
		if _, err := store.Import(ctx, tc.format, strings.NewReader(tc.value)); err == nil {
			t.Fatalf("invalid %s import succeeded", tc.format)
		}
	}
	if _, err := store.Import(ctx, "csv", io.LimitReader(bytes.NewReader(bytes.Repeat([]byte("x"), maxImportBytes+1)), maxImportBytes+1)); err == nil {
		t.Fatal("oversized import succeeded")
	}
}
