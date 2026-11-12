# Limitations

This project currently provides a billing calculation library and local demonstration, not a complete production billing service. The exact-decimal billing package, a three-customer demo, an initial tenant-aware PostgreSQL schema, row-level security policies, and immutable-evidence triggers exist. The HTTP demo is nonpersistent and has no production authentication; it does not apply or connect to PostgreSQL.

The current implementation does not include:

- Persistent customer, plan, subscription, invoice, or API-key services.
- API-key issuance, hashed-key authentication, scopes, and audit recording.
- Database-backed idempotent usage ingestion and storage balance transitions.
- Atomic hard-quota consumption, reservations, and expiration handling.
- Late-event enforcement and the 48-hour finalization grace workflow.
- Subscription creation, upgrades, downgrades, cancellation, and entitlement APIs.
- Adjustments and immutable invoice finalization.
- Runtime database-role provisioning or PostgreSQL RLS integration tests.
- A persistent dashboard; demo simulation recomputes a preview and discards the added quantity.
- Production OpenAPI (the current spec covers only demo routes), browser E2E, scheduled maintenance, and a benchmark harness.
- Tax determination, payment collection, accounting integration, refunds, and currency conversion.

No production security audit or performance claim is made.
