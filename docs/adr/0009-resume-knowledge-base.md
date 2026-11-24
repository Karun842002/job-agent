# ADR-0009: Resume knowledge bases as relevance evidence

- **Status:** Accepted
- **Date proposed:** 2026-11-24
- **Decision owner / approver:** Project owner
- **Approval evidence:** Project conversation, 2026-11-24: “Acceept both ADRs”
- **Related GitHub Issue:** [#19](https://github.com/Karun842002/job-agent/issues/19)
- **Issue ADR backlink:** Issue #19 decisions section
- **Work status:** Accepted; implementation must identify the application database interface and use synthetic fixtures until ingestion is permitted.
- **Affected specification:** FR-25, P1-004, §3.4
- **Supersedes:** None; ADR-0006 retains the approved qualification rule

## Context / problem

Phase 1's inspector currently extracts ATS board identities; it does not provide the geography, title/skills or career evidence needed by the approved relevance rule. Profile and preference schemas are scheduled for later phases.

## Agent–human clarification record

| Date | Question | Human response | Source | Still open? |
|---|---|---|---|---|
| 2026-11-24 | Where should Phase 1 obtain relevance evidence? | “The resume knowledge bases will have all relevant information for job search and resume building.” | Project conversation; earlier context in [issue #19](https://github.com/Karun842002/job-agent/issues/19#issuecomment-7132540004) | Location, format, access and matching interface not yet identified; explicit ADR acceptance pending |

## Options considered

1. Use the resume knowledge bases as the source of candidate information: aligns relevance and resume-building evidence, subject to access and verified facts.
2. Maintain a separate Phase 1 preference/evidence source: duplicates information and risks inconsistencies.

## Proposed decision and rationale

Use the resume knowledge bases as the source of candidate information for deterministic job-search relevance and later resume building. Preserve verified facts; discovery remains zero-model. The owner's answer does not identify the knowledge-base location, format, access mechanism, or how permitted job/source evidence is obtained. Do not invent those interfaces or candidate facts. Acceptance and any material issue reapproval are pending.

## Consequences and migration

Relevance depends on an accessible, appropriately structured source of verified information. Missing or unknown data must remain distinguishable from negative evidence. Avoid duplicating private resume contents in issues, logs, traces or public fixtures. Use synthetic fixtures for verification.

## Requirement, task and verification changes

| Source | Current state | Proposed change | Approval / follow-up |
|---|---|---|---|
| FR-25 / P1-004 | Evidence sources unspecified | Use resume knowledge bases for candidate information | Pending owner acceptance |
| Issue #19 AC1–2 | Approved Boolean qualification rule | Identify source/interface and test deterministic extraction and deduplication | Pending reconciliation and reapproval if material |
| Tests | Boolean and broker fixtures | Add synthetic knowledge-base and job-source fixtures | Pending |

## Resolution and Issue reconciliation

- **Approved/rejected by:** Project owner
- **Date & evidence:** 2026-11-24, project conversation: “Acceept both ADRs”
- **Issue backlink:** Issue #19 decisions section
- **Affected Issue AC revisions and reapproval:** Pending
- **Linked implementation PR:** Pending
- **Supersession note:** None

## Chronological addenda

2026-11-24: The owner clarified: “The database from which the system retrieves information to curate the resume for a given job posting is what im referring to as the resume knowledge base.” The owner also responded “Acceept both ADRs” to the explicit acceptance and issue reconciliation request. This identifies the application’s resume evidence database, without specifying a concrete schema or deployed instance.
