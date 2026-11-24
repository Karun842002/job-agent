# Data classification and lifecycle

The project handles a single candidate's data. Access is limited to authorised
operators and services, with encryption in transit and at rest. Record access
to sensitive material in the audit ledger without copying the material into
logs. Credentials belong in a secret manager and must never enter source code,
logs, traces, prompts, browser evidence or workflow history.

| Data | Classification and access | Retention and deletion |
|---|---|---|
| Profile, CV, answers, contact details and generated application artifacts | Sensitive personal data. Limit access to the candidate and authorised processing components. | Automatically delete 24 hours after creation, including stored copies. |
| Communications and optional email metadata | Sensitive personal data. Use separately consented, least-privilege access. Do not store email bodies by default. | Automatically delete 24 hours after creation, including stored copies. |
| Operational logs and traces, including model telemetry and browser evidence | Diagnostic data that may contain personal information. Restrict access, redact by default, and store screenshots or excerpts only when useful. | Automatically delete 24 hours after creation, including stored copies. |
| Audit events and policy decisions | Protected audit data. Append only during the retention window and restrict read access. | Automatically delete 24 hours after creation, including stored copies. |
| Credentials, tokens, cookies and browser sessions | Secrets. Restrict access to authorised services and use a secret manager for credentials. Never include these in logs or evidence. | Follow the applicable secret/session lifecycle and revoke when no longer required. Personal, diagnostic or audit records containing secrets are subject to the 24-hour policy. |
| Public board and job metadata | Source data. Treat retrieved content as untrusted and enforce source policy and access rules. | Apply the 24-hour policy to any personal, diagnostic or audit records and copies. |

Retention applies to backups and exports that contain the affected records.
Restores must filter expired records before they become accessible. Implement
and test automatic deletion and recovery before ingesting real candidate data.
The selected policy and its effect on audit history are recorded in
[ADR-0003](adr/0003-24-hour-retention.md).

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
  Apply the same expiry to copies, verify cleanup, and prevent expired records
  from being restored. Treat a failure as an operational error requiring action.
- **Unexpected submissions** can affect external sites. Require the specified
  capability checks and explicit user approval for each actual submission.
