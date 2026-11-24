package tiers

import "testing"

func TestRelevant(t *testing.T) {
	base := RelevanceEvidence{Matches, Matches, Matches, Matches, NoMatch}
	tests := []struct {
		name     string
		evidence RelevanceEvidence
		want     bool
	}{
		{"title", base, true},
		{"skills", RelevanceEvidence{Matches, Matches, Matches, NoMatch, Matches}, true},
		{"both", RelevanceEvidence{Matches, Matches, Matches, Matches, Matches}, true},
		{"neither", RelevanceEvidence{Matches, Matches, Matches, NoMatch, NoMatch}, false},
		{"geography", RelevanceEvidence{NoMatch, Matches, Matches, Matches, Matches}, false},
		{"career", RelevanceEvidence{Matches, NoMatch, Matches, Matches, Matches}, false},
		{"source", RelevanceEvidence{Matches, Matches, NoMatch, Matches, Matches}, false},
		{"unknown gate", RelevanceEvidence{Unknown, Matches, Matches, Matches, Matches}, false},
		{"unknown alternatives", RelevanceEvidence{Matches, Matches, Matches, Unknown, Unknown}, false},
		{"invalid gate", RelevanceEvidence{Match(255), Matches, Matches, Matches, Matches}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Relevant(test.evidence); got != test.want {
				t.Fatalf("Relevant(%+v) = %t, want %t", test.evidence, got, test.want)
			}
		})
	}
}
