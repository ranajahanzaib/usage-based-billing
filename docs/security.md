# Security model

## Release boundary

The runnable HTTP application is a local demonstration without authentication, persistence, rate limiting, or a database connection. It requires `DEMO_MODE=1` and defaults to `127.0.0.1:8080`. Keep it on loopback. Do not expose it to an untrusted network or enter real customer data.

The PostgreSQL migration defines forced row-level security on tenant-owned tables. Policies require a transaction-local `app.organization_id`. Production deployment requires a distinct non-owner database role and transaction-local tenant context derived from authenticated credentials. The migration does not provision that role, and PostgreSQL integration tests are not included in the current implementation.

## Production security requirements

- **Tenant isolation:** authenticate API keys, enforce scopes, scope queries by organization, and retain RLS as a second boundary.
- **Credential handling:** generate keys with cryptographic randomness, store secure hashes, reveal each key once, redact authorization headers from logs, and support revocation.
- **Idempotency:** persist unique organization/source/event identities and reject reuse with a different canonical payload.
- **Quota races:** check committed usage and live reservations in one transaction with appropriate row locking or atomic updates.
- **Historical billing:** retain immutable price versions and invoice evidence. Record corrections as separate signed adjustments with an audit record.
- **Timestamp validation:** enforce the configured counter-event lateness policy, use server transaction time for strict storage changes and reservation transitions, and never move rejected events into a different billing period.
- **Resource limits:** bound request bodies, batch sizes, metadata, query ranges, and concurrent work. Apply rate limits at the service or a trusted proxy.
- **Database safety:** use parameterized statements and return generic client errors while keeping structured, redacted diagnostics internally.

No production security audit or certification is claimed. The schema and calculation package alone do not provide a production billing service.
