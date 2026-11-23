# Cron entry points

Build the command with `go build -o /path/to/job-agent ./cmd/job-agent`.
The commands currently exit with an error because their implementations are
pending. Do not install the examples until the discovery, polling and monitoring
implementations are complete and configured.

For Linux or WSL with a running cron service, example entries are:

```cron
*/10 * * * * /path/to/job-agent discover-companies --once
*/10 * * * * /path/to/job-agent poll-boards --once
*/10 * * * * /path/to/job-agent discovery-monitor --once
```

Schedules and paths must be configured for the deployment. Discovery and polling
run independently from the monitor. See the specification §9.1 for run history,
concurrency, monitoring and alerting requirements.
