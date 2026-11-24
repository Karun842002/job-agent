# Architecture Decision Ledger

This file is the **index**, not another requirements specification. Each accepted decision has one immutable-by-default numbered ADR. `PHASED_SPEC.md` remains authoritative for product behaviour; the approved GitHub Issue remains the agreed implementation scope. ADRs document the rationale and human answers that informed changes.

## Ledger

| ID | Decision | Status | Affected spec / issue | Supersedes |
|---|---|---|---|---|
| [ADR-0001](0001-deterministic-cron-discovery.md) | Deterministic cron discovery with zero LLM calls | Accepted (existing product direction) | §1, §2, §7, §9; future implementation Issues | — |
| [ADR-0002](0002-pause-for-clarification-and-link-decisions.md) | Pause on low-confidence decisions; append ADRs and backlink Issues | Accepted (human instruction) | Development workflow; future GitHub Issues | — |

| [ADR-0003](0003-24-hour-retention.md) | Delete personal, diagnostic and audit data 24 hours after creation | Accepted (owner instruction) | §10, §12, §15.10; FR-19; #11 | — |

| [ADR-0004](0004-api-authentication.md) | Server-configured bearer token for the API | Accepted (owner instruction) | §8; FR-01; #16 | — |

| [ADR-0005](0005-provider-validation-permission.md) | Temporary public provider-board validation | Accepted (owner instruction) | §3.1–3.3; Phase 1 exit; FR-03; #21 | — |

| [ADR-0006](0006-relevance-qualification.md) | Deterministic relevance qualification | Accepted (owner instruction) | FR-25; P1-004; #19 | — |

| [ADR-0007](0007-remote-telemetry-retention.md) | Gate remote telemetry on retention compliance | Accepted (owner instruction) | FR-19, FR-30; P0-010; #27 | — |

## Minimal operating rules

1. **Trigger / pause:** When the agent lacks confidence or cannot determine a consequential next step from approved requirements and evidence, **stop affected work and ask the human**. Record the question and `blocked` state on the Issue. When the answer produces a new requirement/design/implementation decision, an ADR and Issue backlink are mandatory. Ordinary clear, reversible local fixes do not need an ADR.
2. **Propose and append:** Create the next `NNNN-short-title.md` from [`0000-template.md`](0000-template.md) with `Status: Proposed`, Issue link, dated human Q&A and requirement impact. **Append a row** to this ledger in numerical order. Preserve the history of accepted ADRs and prior Q&A.
3. **Review:** Agent records questions and quotes/faithfully summarises human responses, linking the original Issue/discussion. Do not invent agreement or treat a suggestion as confirmed.
4. **Accept:** Explicit human sign-off is mandatory for material requirement/architecture changes. Record approver, date and reference, and change status to `Accepted` or `Rejected`.
5. **Reconcile and backlink:** Update the **same GitHub Issue** with an ADR link, brief confirmed decision, affected ACs and approval state; link the Issue in the ADR. The approved change then updates affected `PHASED_SPEC.md` sections and tests/ACs; material scope changes require renewed Issue approval. Link spec ↔ Issue ↔ ADR ↔ PR. Until the issue backlink exists, treat the record as not reconciled.
6. **Supersede:** Never silently rewrite rationale of an accepted ADR. Publish a new ADR referring to the previous ID, mark the old record `Superseded by ADR-NNNN`, and update this index. Minor factual fixes are allowed with an explicit note in Git history.
7. **Unresolved interviews:** Log open questions in the linked GitHub Issue, not speculative decisions. A Proposed ADR can retain a Q&A table while awaiting approval, but does not change the product contract. If GitHub update access is unavailable, prepare the exact intended edit and leave the backlink pending; never report a write that did not occur.

Keep the ledger small: one material decision per ADR, no separate PA agents, databases, or recurring decision-scoring jobs.
