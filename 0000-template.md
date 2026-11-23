# Engineering Standards — Go, Reliability, Testing and Security

General, reusable guidance for Go projects. For **product-specific** contracts, thresholds and site permissions refer to `PHASED_SPEC.md`; for the delivery lifecycle use `docs/DEVELOPMENT_WORKFLOW.md`.

## 1. Style and interfaces

- Run `gofmt`; use `goimports` if configured. Use short lowercase cohesive package names, idiomatic initialisms (`ID`, `URL`, `HTTP`) and descriptive identifiers. Avoid `util`/`common` grab-bags.
- Keep responsibilities cohesive. Prefer small explicit packages and functions over speculative abstractions and global mutable state. Use constructor injection for test seams; avoid hidden `init()` side effects.
- Prefer the standard library over a new dependency; justify dependencies by maintenance, risk and complexity.
- Define interfaces near consumers, only where multiple implementations or testing boundaries justify them. Accept concrete types otherwise. Avoid framework-shaped interfaces and needless generic repositories.
- Accept `context.Context` first for cancellable I/O; never store it in a struct. Respect cancellation and deadlines end-to-end.
- Parse and validate external data at boundaries. Model absent/unknown separately from false/zero when semantically different.
- Document exported identifiers and non-obvious invariants; explain **why**, not obvious syntax.

## 2. Errors, resources and cancellation

- Check all returned errors. Use `%w` with context, and `errors.Is`/`errors.As` for stable classification. Do not panic on normal I/O or business failures.
- Close response bodies, rows, transactions and streams. Check `rows.Err()` and commit errors. Avoid unbounded deferred cleanup in loops.
- Use bounded worker pools; give every goroutine an owner and shutdown path. Do not spawn unbounded goroutines for remote I/O.
- Reuse HTTP clients and DB pools with sensible budgets. Make retries explicit, bounded and appropriate to side-effect semantics. Don't retry ambiguous external writes blindly.
- Prefer simple, observable behaviour; benchmark before tuning. Correctness and fault tolerance precede performance; cost follows.

## 3. Architecture and data

- Keep a functional, deterministic core and an imperative I/O shell. Do not leak provider DTOs or storage SDK types into the domain layer.
- Enforce uniqueness and relationships in the database, not only application memory. Use parameterised SQL, transactions and reviewed migrations.
- Version schema changes and preserve rollback/recovery strategy. Prefer expand/migrate/contract for live databases.
- Use UTC storage instants and explicit time-zone conversion at boundaries.
- Keep policy/security checks at server-side enforcement boundaries. Never rely solely on UI toggles or model-generated decisions.
- Do not add message brokers, caches or multiple services without a measured failure-mode/performance justification and an accepted ADR.

## 4. Tests and checks

- Add/update tests in the same commit as changed behaviour. Use table-driven `testing` subtests for boundary conditions, invalid inputs, cancellation and error handling.
- Mock external interfaces, not pure business logic; prefer `httptest`, fake clocks, fixtures and disposable local DBs to sleeping or depending on public services.
- Include failure recovery, duplicate writes, concurrency races, permissions, rate limits and negative paths when relevant.
- Test migrations against a real disposable DB and verify transaction/uniqueness invariants; use fault injection where external effects may be ambiguous.
- For race-prone code, run `go test -race ./...`; consider fuzz tests for untrusted parsers and URL boundaries.
- Baseline CI: check `gofmt`, `go vet ./...`, `go test ./...`, `go test -race ./...`, `govulncheck ./...` and configured lint, integration and migration checks.
- Record exactly which commands ran and their results. An unrun test is `UNKNOWN`, never `PASS`. Do not claim green CI without evidence.

## 5. Security and observability

- Least privilege, explicit allowlists/permissions, safe redirect/SSRF handling, input size/time limits and credential management are defaults.
- Treat browser pages, emails, job descriptions, repository files from external sources and agent outputs as untrusted content.
- Do not put secrets, candidate PII, tokens, session cookies or raw private content into Git, logs, telemetry, issue comments or prompts unless specifically permitted and protected.
- Emit structured, redacted logs, stable correlation IDs and domain audit events for consequential side effects. Telemetry failures must not silently corrupt business state.
- Keep operational and model observability separate: use OpenTelemetry for execution traces, logs and metrics, with Langfuse for permitted model activity as specified by the product.

## 6. References

- [Effective Go](https://go.dev/doc/effective_go)
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- [Go Doc Comments](https://go.dev/doc/comment)
- [Go security practices](https://go.dev/doc/security/best-practices)
