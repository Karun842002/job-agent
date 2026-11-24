# ADR-0010: Lift retention-based operating gates

- **Status:** Proposed
- **Date proposed:** 2026-11-24
- **Decision owner / approver:** Project owner; explicit acceptance pending
- **Approval evidence:** Pending
- **Related GitHub Issue:** [#27](https://github.com/Karun842002/job-agent/issues/27)
- **Issue ADR backlink:** Issue #27 decisions section
- **Work status:** Affected work blocked pending acceptance and issue reconciliation
- **Affected specification:** FR-19, FR-30, P0-010, §12, §15.10
- **Supersedes:** Proposed replacement of the retention-based remote export and candidate-data gates in ADR-0007; ADR-0008 continues to govern documented retention defaults.

## Context / problem

ADR-0008 removes the strict 24-hour deletion deadline and requires actual configured retention or tool defaults to be documented. ADR-0007 separately disabled remote telemetry and real candidate-data ingestion until retention compliance was verified. Some current defaults have no automatic expiry; external copies may have unknown retention.

## Agent–human clarification record

| Date | Question | Human response | Source | Still open? |
|---|---|---|---|---|
| 2026-11-24 | After accepting ADR-0008, should the remote telemetry and real candidate-data gates remain or be lifted using documented tool defaults, which may retain data indefinitely? | “Lift both gates using documented defaults” | Project conversation; issue [#27](https://github.com/Karun842002/job-agent/issues/27) | Explicit acceptance of this consequential ADR and issue reconciliation pending |

## Options considered

1. Lift both retention-based gates using documented defaults: enables planned functionality but data and copies may persist indefinitely.
2. Keep the gates until a separate retention policy is verified: delays the affected functionality.

## Proposed decision and rationale

Lift the two retention-based operating gates, documenting each backend's actual retention and default behavior under ADR-0008. This does not authorize specific third-party model use, broader data sharing, source access or application submissions; those continue to require their existing permissions and privacy controls. No actual candidate data is required for tests. The owner selected this direction; explicit acceptance of this ADR and any materially changed issue criteria remains pending.

## Consequences and migration

Remote model telemetry may be configured and candidate-data ingestion implemented without a verified 24-hour deletion policy, once other relevant permissions and controls are met. Some stores and backups may retain data indefinitely or have unknown retention; documentation must say so. Re-enabling remote export requires the existing redaction controls and tests. No data or backups are deleted by this decision.

## Requirement, task and verification changes

| Source | Current state | Proposed change | Approval / follow-up |
|---|---|---|---|
| ADR-0007 / FR-30 | Remote export disabled pending a compliant backend | Supersede the retention-specific export gate | Pending |
| ADR-0007 / FR-19 | Candidate-data ingestion blocked pending retention enforcement | Supersede that retention-specific gate | Pending |
| Issue #27 AC3 | Keep both gates | Reconcile to selected direction; verify redaction and documented defaults | Pending reapproval |

## Resolution and Issue reconciliation

- **Approved/rejected by:** Pending (human only)
- **Date & evidence:** Pending
- **Issue backlink:** Issue #27 decisions section
- **Affected Issue AC revisions and reapproval:** Pending
- **Linked implementation PR:** Pending
- **Supersession note:** Preserve ADR-0007 history; change its index status only after acceptance.

## Chronological addenda

None.
