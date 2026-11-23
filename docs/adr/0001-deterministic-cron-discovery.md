# ADR-0001: Use deterministic cron for company discovery and job polling

- **Status:** Accepted (existing product direction)
- **Recorded:** 2026-11-23
- **Decision source:** Previously confirmed project requirement; this record documents the already adopted approach, not a new agent approval
- **Affected specification:** `PHASED_SPEC.md` §1, §2, §7 and §9; `FR-33`, `FR-34`
- **Related Issues:** To be linked when implementation Issues are created
- **Supersedes:** None

## Context

Company and ATS board discovery, public job polling and historical-run monitoring are frequent, predictable, data-driven tasks. Running an LLM or coding agent on every scheduled tick adds cost and unnecessary nondeterminism. A separate monitor is required to detect silent failures and stale sources.

## Decision

Implement discovery, polling and history-monitoring as **bounded, one-shot deterministic Go CLIs run by OS cron**. These paths make **zero LLM calls** and never synchronously launch a browser or coding agent. Difficult dynamic sites become explicit exception records for a separately authorised browser workflow. Retain Temporal for downstream application or human-approval workflows, not polling schedules.

## Alternatives considered

- **Agent-driven every poll:** flexible but incurs token cost and nondeterministic execution; rejected for regular polling.
- **Temporal for scheduled polling:** durable, but unnecessary scheduling infrastructure for simple periodic reads in this deployment; not selected.

## Consequences

- Easier cost prediction, reproducibility and fixture-based testing; no tokens consumed on recurring discovery.
- Must provide cron run history, watchdog monitoring independent of producer process where practical, safe concurrency/leases, rate limits and alerting for missed ticks.
- Dynamic or browser-only sources are not resolved inline and require asynchronous exception handling.

## Clarification and source fidelity

This decision was explicitly provided as a project requirement by the human owner. No verbatim Q&A transcript is included; future changes require a new ADR with dated interview questions, verified answers and approval evidence.

## Linked changes

- Specification: already documents the chosen design.
- Implementation Issues and PRs: pending.
