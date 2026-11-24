# Current local retention behavior

This describes the supplied local Compose configuration, not a guarantee for
other deployments. ADR-0008 supersedes the earlier 24-hour deletion deadline.
Record any operator overrides and re-check them for each deployment. The
separate remote-export and candidate-data gates in ADR-0007 remain in force
pending explicit reconciliation.

| Store or copy | Current behavior | Verification and limits |
|---|---|---|
| PostgreSQL named volume | No automatic expiry is configured for the current tables. The volume survives container restarts. | `compose.yaml`, migrations and the code contain no retention scheduler. Rows can persist indefinitely. An audit trigger permits deletion after 24 hours but does not delete anything itself. |
| Database backups and exports | The documented `pg_dump` drill creates an operator-managed SQL file. The project does not automatically expire that file or other copies. | `docs/DATABASE.md`; retention of external copies is unknown and depends on their storage and operator. The restore drill does not enforce expiry. |
| Local Jaeger traces | The supplied Jaeger all-in-one uses its in-memory backend; trace data is lost on process restart. There is no configured time-based expiry. | Jaeger 1.62.0 `print-config` reports `span-storage.type memory` and `memory.max-traces 0`. See [Jaeger deployment documentation](https://www.jaegertracing.io/docs/1.76/deployment/). |
| Collector and local mock | The Collector forwards operational traces to Jaeger and filtered GenAI traces to a local mock. No persistent Collector queue is configured. | `deploy/otel/collector.yaml`; mock retention depends on the test receiver. Remote export is disabled in the supplied configuration. |
| Container logs | Compose does not set a logging driver or rotation options. The inspected development daemon uses `json-file` with no container-specific options. | `docker info` and a disposable container inspection. The documented `json-file` max-size default is unlimited; daemon overrides or other environments may differ. See [Docker logging documentation](https://docs.docker.com/engine/logging/drivers/json-file/). |
| Application stdout/stderr | The process writes to its caller's streams; retention depends on the supervisor or terminal. | No application log-file retention is configured. The sink and any copies must be assessed for each deployment. |
| Resume evidence, artifacts and email | The planned production stores and their retention are not implemented in this phase. | Unknown until each store is introduced and its settings, backups and deletion behavior are verified. |

No end-to-end deletion or recovery deadline is verified. The PostgreSQL audit
deletion guard is an immutability control, not an automatic cleanup policy.
Backups may contain records removed from the primary database. Document and
verify any future retention configuration and deletion procedure, including
failure and recovery behavior, before claiming that it is enforced.
