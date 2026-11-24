# Job Agent

Job Agent is a Go project for job discovery, CV tailoring, and application workflows. See the [phased specification](PHASED_SPEC.md) for the product scope and [AGENTS.md](AGENTS.md) for the development workflow and decision history.

## Build and test

Install Go 1.26.11 or newer and Make. From a clean checkout:

```sh
make build
make test
```

Both commands use Go's read-only module mode. When adding or updating a dependency, run `go mod tidy` and commit the resulting changes to `go.mod` and `go.sum`.

## Local API

Start the database and apply migrations as described in [docs/DATABASE.md](docs/DATABASE.md). Set `DATABASE_URL` and a high-entropy `API_BEARER_TOKEN` through the environment, then run `go run ./cmd/api`. The API listens on `127.0.0.1:8080` by default. `HTTP_ADDR` overrides the address. Provide TLS and access controls before exposing it beyond loopback.

The company, seed and board endpoints require an `Authorization: Bearer ...` header. The [OpenAPI description](api/openapi.yaml) lists the endpoints. List endpoints return at most 100 records ordered by ID. Use `after` with the last returned ID to request the next page.

For `POST /api/v1/companies/import?format=csv`, supply a CSV header of `name,domain,careers_url` followed by one company per row. For `format=yaml`, supply a single YAML list of objects with those same keys. Each import is atomic and limited to 1 MiB. Discovery URLs are inspected only by the separately policy-gated discovery workflow.

## Registry administration

Build and run `cmd/admin` for one-shot company import and board discovery, validation and listing commands. Discovery and validation use the same persisted source and tier policies as the API. See [docs/ADMIN.md](docs/ADMIN.md) for command syntax and effects.
