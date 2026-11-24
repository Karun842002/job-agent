# ADR-0006: Deterministic relevance qualification

- **Status:** Accepted
- **Date proposed:** 2026-11-24
- **Decision owner / approver:** Project owner
- **Approval evidence:** Project conversation, 2026-11-24: “Accept both”
- **Related GitHub Issue:** [#19](https://github.com/Karun842002/job-agent/issues/19)
- **Issue ADR backlink:** See issue #19 decisions section
- **Work status while uncertain:** Unblocked after issue reconciliation
- **Affected specification:** FR-25, P1-004
- **Supersedes:** None

## Context / problem

The specification lists deterministic prefilters but does not define how they combine. The choice determines which companies and role categories enter the assignment inbox.

## Agent–human clarification record

| Date | Agent question | Human response | Source | Still open? |
|---|---|---|---|---|
| 2026-11-24 | How should the relevance checks combine? | “Geography, evidence, reliable source, and title or skills” | Project conversation, following [issue question](https://github.com/Karun842002/job-agent/issues/19#issuecomment-7121852489) | Explicit ADR acceptance and any material issue reapproval pending |

## Options considered

1. Require geography, evidence, reliable source and either title or skills: balances coverage with the specified filters.
2. Require title and skills: may exclude relevant roles with incomplete information.
3. Another rule: requires owner specification.

## Proposed decision and rationale

Qualify when eligible geography AND career evidence AND reliable source AND (title match OR skills match) hold. Discovery remains deterministic and makes zero model calls. The owner selected this combination; acceptance of this record is pending. Handling unknown inputs remains subject to the existing specification and issue reconciliation.

## Consequences and migration

The rule excludes candidates without sufficient evidence and can include a title-only or skills-only match. Existing assignments are not automatically changed. Rule fixtures should cover each failed gate, both alternatives, unknown values, and deduplicated repeated discovery.

## Requirement, task and verification changes

| Source | Proposed change | Approval / follow-up |
|---|---|---|
| FR-25 / P1-004 | Specify the selected conjunction and disjunction | Pending |
| Issue #19 AC1–2 | Reconcile rule and fixtures | Reapproval pending if material |
| Tests | Prove each gate, deduplication and zero model calls | Pending |

## Resolution and Issue reconciliation

- **Approved/rejected by:** Project owner
- **Date & evidence:** 2026-11-24, project conversation: “Accept both”
- **Issue backlink:** Issue #19 decisions section
- **Affected Issue AC revisions and reapproval:** Owner accepted the requested reconciliation in the project conversation on 2026-11-24.
- **Linked implementation PR:** Pending
- **Supersession note:** None

## Chronological addenda

2026-11-24: The owner responded “Accept both” to the explicit requests to accept ADR-0006 and ADR-0007 and approve the associated issue clarification and specification reconciliation. The earlier proposed wording records the state before acceptance.
