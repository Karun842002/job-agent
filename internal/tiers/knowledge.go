package tiers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// KnowledgeFact is the latest version of a candidate-provided evidence fact.
// Only verified facts may satisfy the deterministic relevance prefilters.
type KnowledgeFact struct {
	Kind  string
	Value string
}

// Observation contains structured facts about one job from an authorized source.
// Empty fields are unknown, not negative evidence.
type Observation struct {
	Location string
	Title    string
	Skills   []string
}

type evidenceQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// LoadKnowledge reads the latest verified version of each fact from the
// evidence database intended for resume curation. It does not log fact values.
func LoadKnowledge(ctx context.Context, db *sql.DB) ([]KnowledgeFact, error) {
	return loadKnowledge(ctx, db)
}

func loadKnowledge(ctx context.Context, q evidenceQuerier) ([]KnowledgeFact, error) {
	rows, err := q.QueryContext(ctx, `SELECT kind,value FROM (
		SELECT DISTINCT ON (fact_key) fact_key,kind,value,verified
		FROM career_evidence ORDER BY fact_key,version DESC
	) latest WHERE verified ORDER BY fact_key`)
	if err != nil {
		return nil, fmt.Errorf("read resume evidence: %w", err)
	}
	defer rows.Close()
	var facts []KnowledgeFact
	for rows.Next() {
		var fact KnowledgeFact
		if err := rows.Scan(&fact.Kind, &fact.Value); err != nil {
			return nil, fmt.Errorf("read resume evidence: %w", err)
		}
		facts = append(facts, fact)
	}
	return facts, rows.Err()
}

// LoadSourceReliability returns an explicit assessment, not a permission to
// fetch the source. Missing assessments remain unknown.
func LoadSourceReliability(ctx context.Context, db *sql.DB, source string) (Match, error) {
	return loadSourceReliability(ctx, db, source)
}

func loadSourceReliability(ctx context.Context, q evidenceQuerier, source string) (Match, error) {
	var reliable bool
	err := q.QueryRowContext(ctx, "SELECT reliable FROM source_reliability WHERE source=$1", source).Scan(&reliable)
	if errors.Is(err, sql.ErrNoRows) {
		return Unknown, nil
	}
	if err != nil {
		return Unknown, fmt.Errorf("read source reliability: %w", err)
	}
	if reliable {
		return Matches, nil
	}
	return NoMatch, nil
}

func normalized(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func compareObserved(facts, observed []string) Match {
	if len(facts) == 0 || len(observed) == 0 {
		return Unknown
	}
	known := false
	for _, value := range observed {
		value = normalized(value)
		if value == "" {
			continue
		}
		known = true
		for _, fact := range facts {
			if normalized(fact) == value {
				return Matches
			}
		}
	}
	if !known {
		return Unknown
	}
	return NoMatch
}

// EvaluateObservation compares exact, case-insensitive normalized values. It
// makes no model calls and never infers missing evidence from an empty field.
func EvaluateObservation(facts []KnowledgeFact, observed Observation, reliability Match) RelevanceEvidence {
	var locations, titles, skills []string
	career := Unknown
	for _, fact := range facts {
		if normalized(fact.Value) == "" {
			continue
		}
		switch fact.Kind {
		case "experience":
			career = Matches
		case "preferred_location":
			locations = append(locations, fact.Value)
		case "target_title":
			titles = append(titles, fact.Value)
		case "skill":
			skills = append(skills, fact.Value)
		}
	}
	return RelevanceEvidence{
		Geography:      compareObserved(locations, []string{observed.Location}),
		CareerEvidence: career,
		ReliableSource: reliability,
		Title:          compareObserved(titles, []string{observed.Title}),
		Skills:         compareObserved(skills, observed.Skills),
	}
}
