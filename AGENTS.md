# ADR-0002: Pause on unclear decisions and reconcile them with GitHub Issues

- **Status:** Accepted (explicit human instruction)
- **Recorded:** 2026-11-23
- **Decision source:** Human instruction in this conversation; this records the already directed process, not independent agent authorisation
- **Related GitHub Issue(s):** None supplied for this workflow instruction; future affected Issues must link their own decision ADRs
- **Affected documents:** `AGENTS.md`, `docs/DEVELOPMENT_WORKFLOW.md`, `docs/adr/README.md`, `docs/adr/0000-template.md`, `.github/ISSUE_TEMPLATE/implementation.md`, `.github/pull_request_template.md`
- **Supersedes:** None

## Context

An agent working from an approved Issue can encounter gaps or unclear moves after implementation has started. Guessing creates requirement drift, and unrecorded human answers disappear when execution context is reset. A simple, durable decision path is necessary without introducing a separate coordinator or database.

## Human instruction and decision

The human directed that agents **pause and ask questions whenever confidence is low or a move is unclear**. When a new consequential decision is confirmed, **append a new ADR** to the ADR ledger and **update the relevant GitHub Issue to reference it**.

This decision is already directed by the human; no GitHub Issue ID or comment URL was supplied, so no existing Issue has been modified or is claimed to be linked to ADR-0002.

## Operating rule

1. Block the uncertain part of the current task and ask specific decision questions with options, implications and a clearly unapproved recommendation.
2. Once confirmed, create a new numbered ADR for any decision changing requirements, ACs, contracts, implementation direction, safety, or non-obvious trade-offs; append it to the ledger with confirmed Q&A and source evidence.
3. Add a reciprocal link and decision summary to **the same GitHub Issue**, identifying affected ACs and approval state.
4. Human approval is required for consequential decisions, and materially changed approved Issues must be reapproved before affected implementation resumes.
5. Treat unavailable Issue write access and inconsistent Issue/spec/ADR references as blockers. Do not invent completion.

## Consequences

- Human clarifications become visible, durable and reviewable at Issue/PR time.
- Unclear work cannot proceed on assumptions; safe independent work can continue.
- More ADRs may be created, but trivial reversible local implementation choices are excluded to keep maintenance bounded.
- Historical accepted decisions are preserved and superseded rather than silently rewritten.
