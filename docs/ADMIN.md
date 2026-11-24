# Registry administration commands

Build the administrative CLI with `go build -o admin ./cmd/admin`. The CLI
requires `DATABASE_URL` and a database migrated through the current schema.
Import files are read locally and are limited to 1 MiB. CSV imports require
`name,domain,careers_url`; YAML imports are a single list of those records.

```sh
DATABASE_URL="$DATABASE_URL" ./admin companies import --format csv --file companies.csv
DATABASE_URL="$DATABASE_URL" ./admin companies import --format yaml --file companies.yaml
DATABASE_URL="$DATABASE_URL" ./admin boards discover --seed 123
DATABASE_URL="$DATABASE_URL" ./admin boards validate --id 456
DATABASE_URL="$DATABASE_URL" ./admin boards list --after 0
```

`companies import` runs atomically and creates UNASSIGNED companies and seeds.
`boards discover` inspects one registered seed and requires a reviewed
`discovery_allowed` source policy for the seed host and any redirect host.
`boards validate` requires `discovery_allowed` for both the provider API host
and exact `board:provider:key` scope. Both commands apply the current global,
tier, step-mode, and site policy before network access and recheck policy before
LIVE state changes. AUDIT stores a preview; OFF performs no provider request.
These commands reuse the API registry services and return stable JSON results.

Company names, seed URLs, board keys, capabilities and provenance are durable
database records. Do not place secrets or personal information in import files
or source URLs. See [the privacy policy](PRIVACY.md) and
[board validation guidance](BOARD_VALIDATION.md).
