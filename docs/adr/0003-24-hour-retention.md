# ADR-0003: Delete personal, diagnostic and audit data after 24 hours

- **Status:** Accepted
- **Date proposed and accepted:** 2026-11-23
- **Decision owner / approver:** Project owner
- **Approval evidence:** The owner selected “Delete all after creation” in the project conversation after specifying “24 hours”. The responses are recorded in [issue #11](https://github.com/Karun842002/job-agent/issues/11).
- **Related GitHub Issue:** [#11](https://github.com/Karun842002/job-agent/issues/11)
- **Affected specification:** §10, §12, §15.10; FR-19, P0-015
- **Supersedes:** None

## Context

The specification requires retention windows to be selected before ingesting
real candidate data. It leaves those windows open. The issue requires documented
privacy and retention defaults.

## Clarification record

On 2026-11-23 the owner answered “24 hours” when asked for retention periods for
personal data, diagnostic logs and traces, and audit events. When asked whether
all three should be automatically deleted 24 hours after creation, including
audit events, the owner selected “Delete all after creation”.

## Decision

Automatically delete each record in all three categories 24 hours after its
creation. Apply the same deadline to copies containing those records, including
backups and exported artifacts. Enforce deletion in each storage system and
account for expiration when restoring data. Until enforcement is implemented and
verified, real candidate data must not be ingested.

## Consequences

The audit ledger remains append-only during its retention window. Audits and
recovery cannot rely on retaining these records beyond 24 hours. Restoration
must not reintroduce expired records. Tests must cover expiration, deletion of
copies and recovery after expiration. Deletion enforcement is implementation
work beyond the documentation deliverable in issue #11.
