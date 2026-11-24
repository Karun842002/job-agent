# ADR-0004: Use a server-configured bearer token for the API

- **Status:** Accepted
- **Date:** 2026-11-23
- **Decision owner:** Project owner
- **Evidence:** The owner selected “Bearer token” in the project conversation in response to the authentication question recorded on [issue #16](https://github.com/Karun842002/job-agent/issues/16).
- **Affected specification:** §8, FR-01, P1-001
- **Related issue:** [#16](https://github.com/Karun842002/job-agent/issues/16)
- **Supersedes:** None

## Context and decision

The single-user API requires authentication for mutations. The specification
left the authentication mechanism open. The owner chose a server-configured
bearer token over a browser session with CSRF protection.

The API accepts the token in the Authorization header. The token is supplied
through runtime configuration, never committed or logged. Cookie-based ambient
authentication is not used. Local HTTP binds to loopback by default. A deployment
that exposes the API beyond loopback must provide TLS and appropriate access
controls.

## Consequences

API clients must explicitly provide the token. Tests must cover missing,
incorrect and valid credentials and ensure mutations create audit events.
The initial single token is a single-user deployment mechanism. A later change
to sessions, multi-user access or token lifecycle needs a separate decision.
