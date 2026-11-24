package tiers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"karun.tech/jobagent/internal/policy"
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

func TestSyntheticKnowledgeToProposalIntegration(t *testing.T) {
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
	host := "kb-fixture-" + suffix + ".example"
	var company, role int64
	if err := db.QueryRowContext(ctx, "INSERT INTO companies(name,domain) VALUES('synthetic',$1) RETURNING id", host).Scan(&company); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "INSERT INTO role_families(name) VALUES('synthetic role') RETURNING id").Scan(&role); err != nil {
		t.Fatal(err)
	}
	defer func() {
		for _, item := range []struct {
			query string
			arg   any
		}{
			{"DELETE FROM tier_proposals WHERE company_id=$1", company},
			{"DELETE FROM role_families WHERE id=$1", role},
			{"DELETE FROM companies WHERE id=$1", company},
			{"DELETE FROM career_evidence WHERE source_ref=$1", suffix},
			{"DELETE FROM source_reliability WHERE source=$1", host},
			{"DELETE FROM source_policies WHERE source=$1", host},
		} {
			if _, err := db.ExecContext(ctx, item.query, item.arg); err != nil {
				t.Error(err)
			}
		}
	}()
	for i, fact := range []KnowledgeFact{{"experience", "synthetic evidence"}, {"preferred_location", "London"}, {"target_title", "Platform Engineer"}, {"skill", "Go"}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO career_evidence(fact_key,kind,value,source_ref,version,verified)
			VALUES($1,$2,$3,$4,1,true)`, fmt.Sprintf("%s-%d", suffix, i), fact.Kind, fact.Value, suffix); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO source_policies(source,discovery_allowed) VALUES($1,true)", host); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO source_reliability(source,reliable,evidence_ref,reviewed_at) VALUES($1,false,'synthetic',now())", host); err != nil {
		t.Fatal(err)
	}
	version := time.Now().UnixNano()
	if _, err := db.ExecContext(ctx, "INSERT INTO policy_versions(version,global_enabled) VALUES($1,true)", version); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO step_defaults(policy_version,step_key,mode) VALUES($1,'tier_proposal','LIVE')", version); err != nil {
		t.Fatal(err)
	}
	store := Store{DB: db}
	in := ProposalInput{CompanyID: company, SourceURL: "https://" + host + "/careers", ExpiresAt: time.Now().Add(time.Hour)}
	obs := Observation{Location: "London", Title: "Platform Engineer", Skills: []string{"Go"}}
	if _, created, err := store.ProposeObservation(ctx, in, obs); err != nil || created {
		t.Fatalf("unreliable source created=%v err=%v", created, err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE source_reliability SET reliable=true WHERE source=$1", host); err != nil {
		t.Fatal(err)
	}
	if _, created, err := store.ProposeObservation(ctx, in, Observation{Title: obs.Title}); err != nil || created {
		t.Fatalf("unknown geography created=%v err=%v", created, err)
	}
	id, created, err := store.ProposeObservation(ctx, in, obs)
	if err != nil || !created || id == 0 {
		t.Fatalf("proposal id=%d created=%v err=%v", id, created, err)
	}
	_, created, err = store.ProposeObservation(ctx, in, obs)
	if err != nil || created {
		t.Fatalf("repeat created=%v err=%v", created, err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM tier_proposals WHERE company_id=$1", company).Scan(&count); err != nil || count != 1 {
		t.Fatalf("proposal count=%d err=%v", count, err)
	}
	var savedEvidence string
	if err := db.QueryRowContext(ctx, "SELECT evidence::text FROM tier_proposals WHERE id=$1", id).Scan(&savedEvidence); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"synthetic evidence", obs.Location, obs.Title, obs.Skills[0]} {
		if strings.Contains(savedEvidence, value) {
			t.Fatal("proposal evidence contains raw candidate or observation values")
		}
	}
	in.RoleFamilyID = &role
	id, created, err = store.ProposeObservation(ctx, in, obs)
	if err != nil || !created || id == 0 {
		t.Fatalf("role proposal id=%d created=%v err=%v", id, created, err)
	}
	_, created, err = store.ProposeObservation(ctx, in, obs)
	if err != nil || created {
		t.Fatalf("repeated role created=%v err=%v", created, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM tier_proposals WHERE role_family_id=$1", role).Scan(&count); err != nil || count != 1 {
		t.Fatalf("role proposal count=%d err=%v", count, err)
	}
	if _, err := store.SetMode(ctx, ModeChange{StepKey: "tier_proposal", Mode: policy.Off}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ProposeObservation(ctx, in, obs); !errors.Is(err, policy.ErrIntakeDenied) {
		t.Fatalf("OFF observation: %v", err)
	}
	if _, err := store.SetMode(ctx, ModeChange{StepKey: "tier_proposal", Mode: policy.Audit}); err != nil {
		t.Fatal(err)
	}
	preview, created, err := store.ProposeObservation(ctx, in, obs)
	if err != nil || created || preview == 0 {
		t.Fatalf("AUDIT preview=%d created=%v err=%v", preview, created, err)
	}
	var previewDiff string
	if err := db.QueryRowContext(ctx, "SELECT diff::text FROM audit_previews WHERE id=$1", preview).Scan(&previewDiff); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"synthetic evidence", obs.Location, obs.Title, obs.Skills[0]} {
		if strings.Contains(previewDiff, value) {
			t.Fatal("audit preview contains raw candidate or observation values")
		}
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM tier_proposals WHERE role_family_id=$1", role).Scan(&count); err != nil || count != 1 {
		t.Fatalf("AUDIT changed role proposals count=%d err=%v", count, err)
	}
}
