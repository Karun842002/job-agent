# Cron entry points

Build the command with `go build -o /path/to/job-agent ./cmd/job-agent`.
Without a fixture, commands exit with an error because their production
implementations are pending. Do not install the examples until discovery,
polling and monitoring are complete and configured.

For offline verification, pass `--fixture /path/to/fixture.json` after
`--once`. The JSON object accepts a `companies` array, an `as_of` RFC 3339
timestamp, `max_age_seconds`, and a `runs` array with `job`, `outcome`,
and `finished_at` fields. The synthetic monitor checks for a recent successful
discovery and polling run. These fixture checks perform no network access and
are not a production run-history monitor.

For Linux or WSL with a running cron service, example entries are:

```cron
*/10 * * * * /path/to/job-agent discover-companies --once
*/10 * * * * /path/to/job-agent poll-boards --once
*/10 * * * * /path/to/job-agent discovery-monitor --once
```

Schedules and paths must be configured for the deployment. Discovery and polling
run independently from the monitor. See the specification §9.1 for run history,
concurrency, monitoring and alerting requirements.
