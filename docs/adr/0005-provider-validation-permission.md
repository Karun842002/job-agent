# ADR-0005: Temporary public provider-board validation

- **Status:** Accepted
- **Date:** 2026-11-24
- **Decision owner:** Project owner
- **Evidence:** The owner selected “Yes, validate these boards” in the project conversation after the exact hosts and boards were presented. The clarification is recorded on [issue #21](https://github.com/Karun842002/job-agent/issues/21).
- **Affected specification:** §3.1–3.3, Phase 1 exit criterion 1, FR-03
- **Related issue:** [#21](https://github.com/Karun842002/job-agent/issues/21)
- **Supersedes:** None

## Context and decision

Phase 1 requires a real public board to be validated on each initial provider. The source-policy registry requires permission before network reads. The owner authorised temporary discovery/read permissions for `boards-api.greenhouse.io` and `api.ashbyhq.com` to validate the providers' own `greenhouse` and `ashby` boards.

## Consequences

The permission covers these two validation checks. It does not grant application submission or blanket permission for other sources. Temporary test policies and fixture records must be removed after verification. Record only redacted outcomes and avoid storing job descriptions or personal information.
