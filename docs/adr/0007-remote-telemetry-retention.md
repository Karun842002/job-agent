# ADR-0007: Gate remote telemetry on retention compliance

- **Status:** Accepted
- **Date proposed:** 2026-11-24
- **Decision owner / approver:** Project owner
- **Approval evidence:** Project conversation, 2026-11-24: “Accept both”
- **Related GitHub Issue:** [#27](https://github.com/Karun842002/job-agent/issues/27)
- **Issue ADR backlink:** See issue #27 decisions section
- **Work status while uncertain:** Unblocked after issue reconciliation
- **Affected specification:** FR-19, FR-30, P0-010, §12, §15.10
- **Supersedes:** None; ADR-0003 retains its 24-hour policy

## Context / problem

The required Langfuse export must meet ADR-0003's 24-hour deletion rule. [Langfuse retention documentation](https://langfuse.com/docs/administration/data-retention) describes a three-day minimum and nightly cleanup; project retention excludes audit logs. This leaves a conflict between the current remote export and the approved policy.

## Agent–human clarification record

| Date | Agent question | Human response | Source | Still open? |
|---|---|---|---|---|
| 2026-11-24 | How should remote telemetry reconcile with the 24-hour policy? | “Disable remote export pending a compliant backend” | Project conversation; evidence recorded on [issue #27](https://github.com/Karun842002/job-agent/issues/27#issuecomment-7122267290) | Explicit ADR acceptance and any material issue reapproval pending |

## Options considered

1. Disable remote export until a compliant backend is selected and verified: preserves the 24-hour policy but delays remote model observability.
2. Select a different backend now: requires a verified implementation and retention guarantees for all copies.
3. Change the retention policy: would change ADR-0003 and requires a separate decision.

## Proposed decision and rationale

Disable remote telemetry export pending selection and verification of a backend that satisfies the 24-hour policy. Keep synthetic local mock validation available. The exact enforcement mechanism and backend remain to be verified. The owner selected this direction; acceptance of this record is pending.

## Consequences and migration

Remote Langfuse model traces are unavailable while the gate is in place. Keep real candidate data ingestion blocked until all applicable stores, copies and recovery paths satisfy the retention policy. Restoring remote export requires verified deletion and reconciliation with FR-30.

## Requirement, task and verification changes

| Source | Proposed change | Approval / follow-up |
|---|---|---|
| FR-30 / P0-010 | Gate remote export while retaining synthetic mock verification | Pending |
| Issue #27 AC1–4 | Reconcile external export gate with deletion and recovery criteria | Reapproval pending if material |
| Tests | Verify gate, deletion, copies, failures and recovery | Pending |

## Resolution and Issue reconciliation

- **Approved/rejected by:** Project owner
- **Date & evidence:** 2026-11-24, project conversation: “Accept both”
- **Issue backlink:** Issue #27 decisions section
- **Affected Issue AC revisions and reapproval:** Owner accepted the requested reconciliation in the project conversation on 2026-11-24.
- **Linked implementation PR:** Pending
- **Supersession note:** None

## Chronological addenda

2026-11-24: The owner responded “Accept both” to the explicit requests to accept ADR-0006 and ADR-0007 and approve the associated issue clarification and specification reconciliation. The earlier proposed wording records the state before acceptance.
