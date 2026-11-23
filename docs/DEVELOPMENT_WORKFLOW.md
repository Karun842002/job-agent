# Development Workflow — Minimal Requirements and Code Quality Gates

This document defines reusable **development process**, not job-application business logic. `PHASED_SPEC.md` is the product specification; the current human-approved GitHub Issue is the implementation contract. GitHub Issues, linked PRs and accepted ADRs are the durable records. **No PA agents, task coordinator, lease system or separate tracking database.**

## 1. Issue lifecycle and ownership

`Draft Issue → Quality review → Revise (≤3 rounds) → Human approves Issue → Implement & test → Code verification → Fix (≤3 rounds) → Create PR → Human approves → Human merges`

**Interrupt at any stage:** If the agent lacks confidence or a requirement/next move is unclear, pause affected work, mark the Issue `blocked`, ask targeted questions, and follow §3 before resuming. No score can replace a confirmed decision.

Recommended issue labels: `needs-quality-review`, `needs-revision`, `awaiting-issue-approval`, `approved-for-implementation`, `in-progress`, `blocked`. Scores and each review's evidence go in the Issue comments; there is no duplicate state ledger.

- Use `.github/ISSUE_TEMPLATE/implementation.md` to create one narrowly scoped Issue.
- Give each requirement an ID (`FR-*`) and each phase deliverable a reference (`Pn-NNN`) where applicable.
- GitHub Issue acceptance criteria are authoritative **for an approved task**, but they must be consistent with `PHASED_SPEC.md`. Escalate disagreement; do not silently choose either side.
- Only a **human approval comment** and human-applied/confirmed `approved-for-implementation` label permit coding to start. Agent-created labels or its own comments do not count.

## 2. Requirements quality rubric (advisory 0–100)

Give each dimension a score from 0 to 5. Missing content scores zero. The combined result is:

`15 * purpose / 5 + 25 * scope / 5 + 25 * technical / 5 + 35 * acceptance / 5`

| Dimension | Weight | Check |
|---|---:|---|
| Value / purpose | 15% | Desired outcome and why it matters |
| Scope and boundaries | 25% | In-scope, out-of-scope, dependencies, assumptions |
| Technical requirements | 25% | Constraints, interfaces, failure/recovery, security as relevant |
| Acceptance criteria and tests | 35% | Objective positive and negative scenarios, verification evidence |

**Ready for human approval:** ≥85/100 overall, ≥3/5 in each dimension, ≥4/5 for acceptance criteria and no blocking ambiguity, missing criteria, contradiction or unapproved external effect. An LLM score is never human authorisation.

Comment on the Issue with: scored dimensions, reasons, specific improvements, blocking questions, revision/attempt number. Amend the **existing Issue**, not a duplicate. Rescore up to three automated rounds; then stop for human clarification rather than inventing missing behaviour.

## 3. Stop, clarify, and append decisions

**Mandatory uncertainty gate:** At any stage, an unclear or low-confidence consequential move is a **stop condition**, not a prompt to guess. Examples: contradicting requirements, ambiguous ACs, non-obvious implementation alternatives, unclear provider permissions, missing migration/rollback guarantees or uncertain safety consequences. Pause only the affected work; label its Issue `blocked`. A high rubric score does not waive this rule.

1. **Ask:** Post a concise question in the existing GitHub Issue or ask the human directly, citing the affected AC, missing fact, viable options, impact of each, and a clearly unapproved recommendation. Do not invent an answer or repeat an already resolved question.
2. **Capture:** When the human answers, preserve the exact decision, date, source and any remaining uncertainties. If a new decision changes requirement interpretation, scope, ACs, contracts, implementation direction or a consequential trade-off, create the next numbered ADR from `docs/adr/0000-template.md`. Add a new chronological row to `docs/adr/README.md`; do not overwrite an earlier accepted decision.
3. **Backlink:** Update **the same Issue** under `Decisions / ADR references` with `ADR-NNNN`, a link, brief decision, impacted ACs and `Proposed`/`Accepted` state. In the ADR, link back to the Issue and discussion. If the Issue cannot be modified, prepare the proposed text and leave the task blocked/pending update.
4. **Approve and reconcile:** The human must explicitly accept a consequential ADR and reapprove any materially changed, previously approved Issue. Only then update the spec, Issue ACs, tests and approval references consistently. A question answered does not necessarily constitute authorisation to change approved scope.
5. **Resume:** Re-read the updated Issue and accepted ADRs before resuming code or submitting a PR. Missing backlinks, unresolved questions, conflicting spec/Issue/ADR statements and pending approval are blockers.
6. **Supersede:** For changed decisions, create a new ADR citing the old one; mark the original superseded in the index without erasing original rationale. Append dated Q&A and subsequent clarifications rather than rewriting confirmed answers.

Minor, obvious, reversible implementation details need no ADR if they do not resolve a disputed requirement. Otherwise, a confirmed decision from this escalation must not exist only in transient agent conversation.

**No auto-approval:** An agent may draft an ADR and prepare spec/Issue edits, but cannot self-approve. Do not claim a GitHub Issue was updated if the write failed or was unavailable.

## 4. Implement, verify and iterate

Use branch `issue/<number>-<short-slug>`. Work only within the approved Issue's scope. Tests accompany implementation. Diff and test claims must be inspected, not inferred from an agent's completion summary.

| Verification dimension | Weight | Evidence |
|---|---:|---|
| Acceptance criteria conformance | 40% | `AC1…ACn` `PASS`/`FAIL`/`UNKNOWN` and code/test references |
| Test adequacy | 25% | Actually executed tests, negative/failure cases |
| Reliability/failure handling | 15% | Retries, cancellation, idempotency, recovery where relevant |
| Go & clean code | 10% | Correct interfaces, errors, cohesion, diff focus |
| Security/policy/operability | 10% | Approvals, privacy, traceability, site permissions |

**PR-ready:** ≥90/100, every acceptance criterion `PASS`, required CI checks green, no high-confidence critical defect or unapproved effect. `UNKNOWN` is not `PASS`. Overall score never overrides a failed acceptance criterion. Provide file/line-specific actionable findings with risk and confidence; avoid unsupported speculation.

After each fix, rerun relevant tests and the review. Stop after three unsuccessful review/fix rounds; report the blocker in the Issue. A requirement change returns to §3 and must receive human approval.

## 5. Single-responsibility commits and PR rules

- Stage selectively (`git add -p` where useful), inspect `git diff --cached`, and check `git status`. Do not stage secrets, personal data or unrelated edits.
- Each commit has exactly **one coherent intent** that can be understood and independently reverted; include the tests for that intent in the same commit. Do not split code and its tests into intentionally broken commits.
- No unrelated refactors, formatting, generated files or dependency churn in the same commit. Split independent work into independent Issues/PRs.
- Use imperative subjects, preferably ≤72 characters, with an Issue reference: `feat(ats): add Ashby normalisation (#123)`.
- A PR can contain **multiple related atomic commits**. Do not enforce one-commit-per-PR if it damages clarity.
- Do not amend/rebase/force-push shared history without authorisation.
- Open the PR **after** verifying all acceptance criteria. Check that each confirmed consequential decision is recorded in a linked, accepted ADR and cross-referenced in the Issue. Use `.github/pull_request_template.md`, link Issue and ADRs, include criterion-by-criterion evidence, commands and actual outcomes, risks/rollback, and follow-ups.
- The agent must **never** approve or merge its PR, enable auto-merge or bypass branch protections. Human approval and manual merge are mandatory; any changed code after approval may require another review.
- Consider the Issue delivered only after the PR is merged. Deferred scope remains a separately linked open Issue.

## 6. Completion and migration checks

- Phase readiness is a manual check of the phase's unchecked `Pn-NNN` deliverables, unresolved linked Issues, unmerged PRs and blocked acceptance criteria. An unbuilt feature is never marked delivered merely because its Issue was closed accidentally.
- Keep standard CI; retire any previous `.codex/hooks.json`, task leases, task-coverage ledger, PA agents and bespoke orchestration scripts **only if present and unrelated work is preserved**.
- These reviews consume LLM tokens only when invoked by a development task. **Product cron discovery, board polling and run-history monitoring must not call a model.**

## 7. Relocated legacy workflow requirements (traceability)

The previous product specification mixed development-process requirements into its product feature list. The requirements below are **relocated, not cancelled**. Existing GitHub Issues may continue citing their old IDs; update links when convenient, without renumbering product `FR-01`–`FR-34`.

| Previous ID | Process responsibility now documented here |
|---|---|
| `FR-35` / `DEV-35` | Structured GitHub Issues, quality scoring, revision loop and human Issue approval (§1–§2) |
| `FR-36` / `DEV-36` | Code-to-acceptance verification, actionable findings and iterative fixes (§4) |
| `FR-37` / `DEV-37` | Single-responsibility commits, PR structure, manual review and merge (§5) |
| `P0-005` | GitHub Issue template: `.github/ISSUE_TEMPLATE/implementation.md` |
| `P0-006` | Requirements scoring and human approval rules (§2) |
| `P0-007` | Verification rubric and PR template (§4–§5) |
| `P0-008` | Protected branch/CI checks and human PR approval/merge (§5) |
| `P0-017` | Root `AGENTS.md` adopted, ADR index and template established (§3) |

The product specification continues to contain the **business and runtime requirements**, while these process requirements are owned here. Any open Issue referring to a legacy process ID should link this section and retain its current acceptance criteria.
