# Data classification and lifecycle

The project handles a single candidate's data. Access is limited to authorised
operators and services, with encryption in transit and at rest. Record access
to sensitive material in the audit ledger without copying the material into
logs. Credentials belong in a secret manager and must never enter source code,
logs, traces, prompts, browser evidence or workflow history.

| Data | Classification and access | Retention and deletion |
|---|---|---|
| Profile, CV, answers, contact details and generated application artifacts | Sensitive personal data. Limit access to the candidate and authorised processing components. | Document the actual configured retention or tool default, including copies; no 24-hour deletion guarantee. |
| Communications and optional email metadata | Sensitive personal data. Use separately consented, least-privilege access. Do not store email bodies by default. | Document the actual configured retention or tool default, including copies; no 24-hour deletion guarantee. |
| Operational logs and traces, including model telemetry and browser evidence | Diagnostic data that may contain personal information. Restrict access, redact by default, and store screenshots or excerpts only when useful. | Document the actual configured retention or tool default, including copies; no 24-hour deletion guarantee. |
| Audit events and policy decisions | Protected audit data. Append only during the retention window and restrict read access. | Document the actual configured retention or tool default, including copies; no 24-hour deletion guarantee. |
| Credentials, tokens, cookies and browser sessions | Secrets. Restrict access to authorised services and use a secret manager for credentials. Never include these in logs or evidence. | Follow the applicable secret/session lifecycle and revoke when no longer required. Apply the documented retention of the relevant store to records containing secrets. |
| Public board and job metadata | Source data. Treat retrieved content as untrusted and enforce source policy and access rules. | Document actual retention for personal, diagnostic or audit records and copies. |

Document retention of backups and exports as well as primary stores. Tool defaults
may be indefinite and must not be represented as a deletion deadline.
[ADR-0008](adr/0008-retention-defaults.md) supersedes the strict 24-hour policy
in ADR-0003. The separate remote export and real candidate ingestion gates in
ADR-0007 remain pending explicit reconciliation.
The [local retention inventory](RETENTION.md) records current known defaults
and gaps; it does not claim an automatic deletion deadline.

## Threat model

- **External pages and email** may contain instructions or hostile content.
  Treat them as data, sanitise rendered HTML, and never permit source text to
  change permissions, invoke tools, or disclose secrets.
- **Unauthorised access** can expose candidate data or credentials. Apply least
  privilege, encryption, access auditing and explicit provider permissions.
- **Browser actions and form autosave** can cause unwanted remote writes. Gate
  every action by policy and keep audit mode free of remote writes and
  production-domain mutations.
- **Logs, model traces and screenshots** can leak inputs. Redact personal data,
  tokens, cookies, browser profiles, form values and full prompts by default.
  Capture raw model inputs or outputs only after explicit separate opt-in.
- **Expired backups or failed deletion** can restore or retain sensitive data.
  Document retention of copies, verify any configured cleanup, and avoid
  representing tool defaults as guaranteed deletion. Treat deletion failures as
  operational errors requiring action.
- **Unexpected submissions** can affect external sites. Require the specified
  capability checks and explicit user approval for each actual submission.
