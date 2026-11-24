package tiers

// Match is a deterministic evidence result. Unknown remains distinct from a
// negative result so callers can explain why a proposal was not created.
type Match uint8

const (
	Unknown Match = iota
	NoMatch
	Matches
)

// RelevanceEvidence holds the prefilter results for a company or role.
type RelevanceEvidence struct {
	Geography      Match
	CareerEvidence Match
	ReliableSource Match
	Title          Match
	Skills         Match
}

// Relevant reports whether the evidence meets ADR-0006. Missing or invalid
// evidence cannot satisfy a required check.
func Relevant(e RelevanceEvidence) bool {
	return e.Geography == Matches && e.CareerEvidence == Matches && e.ReliableSource == Matches &&
		(e.Title == Matches || e.Skills == Matches)
}
