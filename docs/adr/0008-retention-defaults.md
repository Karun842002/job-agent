# ADR-0008: Retention defaults and the 24-hour gate

- **Status:** Proposed
- **Date proposed:** 2026-11-24
- **Decision owner / approver:** Project owner; acceptance pending
- **Approval evidence:** Pending
- **Related GitHub Issue:** [#27](https://github.com/Karun842002/job-agent/issues/27)
- **Issue ADR backlink:** Issue #27 decisions section
- **Work status:** Blocked pending acceptance and issue reconciliation
- **Affected specification:** FR-19, FR-30, P0-010, §10, §12, §15.10
- **Supersedes:** Proposed replacement of the strict 24-hour policy in ADR-0003; effects on ADR-0007 remain subject to owner confirmation.

## Context / problem

ADR-0003 requires all personal, diagnostic and audit records and copies to be deleted within 24 hours. This is not guaranteed by the current local deployment if the machine or database is unavailable. ADR-0007 gates remote telemetry until a compliant backend is verified.

## Agent–human clarification record

| Date | Question | Human response | Source | Still open? |
|---|---|---|---|---|
| 2026-11-24 | How should retention work when a local machine is unavailable at the deadline? | “Lets not care too much about TTLs for now, lets use tool defaults wherever configuration isnt possible.” | Project conversation; earlier context in [issue #27](https://github.com/Karun842002/job-agent/issues/27#issuecomment-7133023597) | Explicit acceptance and specific treatment of remote telemetry and candidate data pending |

## Options considered

1. Remove the strict 24-hour guarantee and use available tool defaults where retention cannot be configured: simpler operations but records can persist longer, potentially indefinitely.
2. Keep the strict 24-hour deadline with independent enforcement: stronger deadline but additional infrastructure and operational complexity.

## Proposed decision and rationale

Treat the strict 24-hour deletion deadline as no longer a Phase 0/1 delivery requirement. Document actual configured or tool-default retention for each deployed store, including whether a default is indefinite. Use tool defaults where configuration is unavailable, following the owner's direction. Do not represent defaults as a verified deletion deadline. The statement does not by itself specify whether ADR-0007's remote-export gate or the candidate-data ingestion gate should be lifted; those remain pending explicit reconciliation.

## Consequences and migration

Existing data and copies can remain longer than 24 hours; restoration may reintroduce data retained under tool defaults. Privacy classification, access controls, redaction and source permissions continue to apply. Avoid changing existing data or deleting backups as part of this decision. Update the specification, issue criteria and tests only after owner acceptance.

## Requirement, task and verification changes

| Source | Current state | Proposed change | Approval / follow-up |
|---|---|---|---|
| ADR-0003 / FR-19 / §15.10 | Strict 24-hour deletion and ingestion gate | Replace strict deadline with documented actual retention/defaults | Pending owner acceptance |
| ADR-0007 / FR-30 | Remote export disabled until compliant backend | Owner direction on whether to retain or lift gate needed | Pending |
| Issue #27 AC1–4 | Strict deletion and restore checks | Reconcile to accepted policy and test actual configuration | Pending reapproval |

## Resolution and Issue reconciliation

- **Approved/rejected by:** Pending (human only)
- **Date & evidence:** Pending
- **Issue backlink:** Issue #27 decisions section
- **Issue reapproval:** Pending
- **Linked implementation PR:** Pending
- **Supersession note:** Pending; preserve ADR-0003 and ADR-0007 history.

## Chronological addenda

None.
