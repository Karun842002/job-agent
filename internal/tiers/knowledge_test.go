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

func TestEvaluateObservation(t *testing.T) {
	facts := []KnowledgeFact{{"experience", "synthetic experience"}, {"preferred_location", "London"}, {"target_title", "Platform Engineer"}, {"skill", "Go"}}
	for _, test := range []struct {
		name        string
		observation Observation
		reliability Match
		want        bool
	}{
		{"title", Observation{Location: " LONDON ", Title: "platform  engineer"}, Matches, true},
		{"skill", Observation{Location: "London", Title: "other", Skills: []string{"go"}}, Matches, true},
		{"wrong location", Observation{Location: "Paris", Title: "Platform Engineer"}, Matches, false},
		{"unknown location", Observation{Title: "Platform Engineer"}, Matches, false},
		{"unknown source", Observation{Location: "London", Title: "Platform Engineer"}, Unknown, false},
		{"denied source", Observation{Location: "London", Title: "Platform Engineer"}, NoMatch, false},
		{"no title or skill", Observation{Location: "London"}, Matches, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := EvaluateObservation(facts, test.observation, test.reliability)
			if Relevant(got) != test.want {
				t.Fatalf("result %+v, qualified=%v, want %v", got, Relevant(got), test.want)
			}
		})
	}
	if got := EvaluateObservation(nil, Observation{}, Unknown); got != (RelevanceEvidence{}) {
		t.Fatalf("missing data must remain unknown: %+v", got)
	}
}

func TestLoadKnowledgeVersionsIntegration(t *testing.T) {
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
	key := fmt.Sprintf("synthetic-%d", time.Now().UnixNano())
	defer func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM career_evidence WHERE fact_key=$1", key); err != nil {
			t.Error(err)
		}
	}()
	for _, row := range []struct {
		version  int
		verified bool
		value    string
	}{{1, true, "old synthetic value"}, {2, false, "unverified replacement"}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO career_evidence(fact_key,kind,value,source_ref,version,verified)
			VALUES($1,'skill',$2,'fixture',$3,$4)`, key, row.value, row.version, row.verified); err != nil {
			t.Fatal(err)
		}
	}
	facts, err := LoadKnowledge(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range facts {
		if fact.Value == "old synthetic value" || fact.Value == "unverified replacement" {
			t.Fatalf("superseded or unverified evidence was returned: %+v", fact)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO career_evidence(fact_key,kind,value,source_ref,version,verified)
		VALUES($1,'skill','verified synthetic value','fixture',3,true)`, key); err != nil {
		t.Fatal(err)
	}
	facts, err = LoadKnowledge(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fact := range facts {
		if fact.Value == "verified synthetic value" {
			found = true
		}
	}
	if !found {
		t.Fatal("latest verified fact was not returned")
	}
	if _, err := db.ExecContext(ctx, "UPDATE career_evidence SET value='changed' WHERE fact_key=$1", key); err == nil {
		t.Fatal("immutable evidence was updated")
	}
	if got, err := LoadSourceReliability(ctx, db, "missing.synthetic.example"); err != nil || got != Unknown {
		t.Fatalf("missing reliability = %v, %v", got, err)
	}
	host := key + ".example"
	if _, err := db.ExecContext(ctx, "INSERT INTO source_policies(source,discovery_allowed) VALUES($1,true)", host); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM source_reliability WHERE source=$1", host); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM source_policies WHERE source=$1", host); err != nil {
			t.Error(err)
		}
	}()
	if got, err := LoadSourceReliability(ctx, db, host); err != nil || got != Unknown {
		t.Fatalf("permission alone must not imply reliability: %v, %v", got, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO source_reliability(source,reliable,evidence_ref,reviewed_at)
		VALUES($1,true,'synthetic-review',now())`, host); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadSourceReliability(ctx, db, host); err != nil || got != Matches {
		t.Fatalf("reliable assessment = %v, %v", got, err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE source_reliability SET reliable=false WHERE source=$1", host); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadSourceReliability(ctx, db, host); err != nil || got != NoMatch {
		t.Fatalf("unreliable assessment = %v, %v", got, err)
	}
}
