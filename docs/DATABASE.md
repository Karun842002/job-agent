# Local database

The Compose configuration is for local development. It binds PostgreSQL to
loopback and uses a development-only password. Run Docker Compose from the
repository root:

```sh
docker compose up -d postgres
docker compose --profile tools run --rm migrate up
```

The named volume preserves data when the container restarts. Migration files
are versioned pairs applied by golang-migrate. Apply new migrations before
running code that requires them. Inspect migration status with:

```sh
docker compose --profile tools run --rm migrate version
```

## Backup and restore drill

Use a disposable local database for this drill. The restore command replaces
objects in the target database. Review the target before running it, and keep
the backup file outside version control because it may contain personal data.

```sh
docker compose exec -T postgres pg_dump -U jobagent -d jobagent --clean --if-exists > jobagent-backup.sql
docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U jobagent -d jobagent < jobagent-backup.sql
```

For a disposable database, the most recent migration can also be rolled back
and reapplied with `docker compose --profile tools run --rm migrate down 1`
followed by `docker compose --profile tools run --rm migrate up`. Rolling back
can delete data. Take and verify a backup before changing a database that
contains anything valuable.
