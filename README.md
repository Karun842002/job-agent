# Job Agent

Job Agent is a Go project for job discovery, CV tailoring, and application workflows. See the [phased specification](PHASED_SPEC.md) for the product scope and [AGENTS.md](AGENTS.md) for the development workflow and decision history.

## Build and test

Install Go 1.22.2 or newer and Make. From a clean checkout:

```sh
make build
make test
```

Both commands use Go's read-only module mode. When adding or updating a dependency, run `go mod tidy` and commit the resulting changes to `go.mod` and `go.sum`. The module currently has no external Go dependencies.
