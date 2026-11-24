package company

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestAuthorisedPublicBoards(t *testing.T) {
	if os.Getenv("LIVE_PUBLIC_BOARD_TEST") != "authorised-provider-owned-boards" {
		t.Skip("live provider-owned board test requires explicit owner authorisation")
	}
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Fatal("TEST_DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	company, err := (Store{DB: db}).CreateCompany(ctx, Company{Name: "Public board validation fixture", Domain: "public-board-fixture-" + suffix + ".example"})
	if err != nil {
		t.Fatal(err)
	}
	var hosts []string
	t.Cleanup(func() {
		cleanup, e := sql.Open("pgx", databaseURL)
		if e != nil {
			t.Error(e)
			return
		}
		defer cleanup.Close()
		for _, host := range hosts {
			if _, e := cleanup.ExecContext(context.Background(), "DELETE FROM source_policies WHERE source=$1", host); e != nil {
				t.Error(e)
			}
		}
		if _, e := cleanup.ExecContext(context.Background(), "DELETE FROM ats_boards WHERE company_id=$1", company.ID); e != nil {
			t.Error(e)
		}
		if _, e := cleanup.ExecContext(context.Background(), "DELETE FROM companies WHERE id=$1", company.ID); e != nil {
			t.Error(e)
		}
	})
	for _, tc := range []struct{ provider, host, key string }{
		{"greenhouse", "boards-api.greenhouse.io", "greenhouse"},
		{"ashby", "api.ashbyhq.com", "ashby"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			result, err := db.ExecContext(ctx, "INSERT INTO source_policies (source,discovery_allowed,evidence,last_reviewed_at) VALUES ($1,true,$2,now()) ON CONFLICT DO NOTHING", tc.host, "owner-authorised temporary public board check")
			if err != nil {
				t.Fatal(err)
			}
			count, err := result.RowsAffected()
			if err != nil || count != 1 {
				t.Fatalf("existing policy for %s; refusing to replace it", tc.host)
			}
			hosts = append(hosts, tc.host)
			board, err := (Store{DB: db}).CreateBoard(ctx, Board{CompanyID: company.ID, Provider: tc.provider, Key: tc.key})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/boards/%d/validate", board.ID), nil)
			request.Header.Set("Authorization", "Bearer fixture-token")
			response := httptest.NewRecorder()
			Handler(Store{DB: db}, "fixture-token").ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("live board validation returned %d", response.Code)
			}
			var resultData ValidationResult
			if err := json.Unmarshal(response.Body.Bytes(), &resultData); err != nil {
				t.Fatal(err)
			}
			persisted, err := (Store{DB: db}).GetBoard(ctx, board.ID)
			if err != nil || persisted.Metadata.State != "verified" {
				t.Fatalf("board state %q: %v", persisted.Metadata.State, err)
			}
			t.Logf("%s public board verified; listing counts available: %t", resultData.Provider, resultData.Listed != nil)
		})
	}
}
