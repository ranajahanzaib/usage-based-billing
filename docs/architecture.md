# Architecture

## Components

The release contains a Go calculation package in `internal/billing`, a small `net/http` demonstration server in `cmd/server`, and a PostgreSQL schema in `db/migrations`. The server exposes health, a three-customer demo list, invoice previews, and stateless counter simulations. It does not connect to PostgreSQL. The HTML demo is served by the Go process.

The calculation package accepts immutable plan values, subscription segments, usage events, storage history, a UTC period, and an explicit calculation time. It has no database dependency. Money and rates enter as decimal strings; rational arithmetic handles proration, graduated tiers, and storage accrual; invoice lines round half up to cents. Counter usage and allowances are calculated per plan segment. Storage byte-seconds and pricing are also calculated per historical plan segment.

## PostgreSQL schema

PostgreSQL is the intended transactional source of truth for a production service. The migration defines organizations, customers, metrics, immutable plan versions, subscriptions and segments, counter and storage evidence, quota and reservation state, invoice records, adjustments, and audit records.

Composite foreign keys prevent cross-organization associations. A GiST exclusion constraint prevents overlapping segments for a subscription. A partial unique index permits one pending subscription change per subscription. Unique event identities support database-enforced deduplication. Triggers prevent updates and deletes to pricing versions, usage evidence, finalized invoices, invoice lines, adjustments, and audit records.

Tenant tables have forced row-level security policies based on the transaction-local `app.organization_id` setting. The schema does not create a runtime role. A production application must use a distinct non-owner role, derive tenant context from authenticated credentials, and set it transaction-locally for every request. The demo server does not use these policies because it does not connect to the database.

## Production service boundary

The demo is intentionally separate from the database schema. The production API, API-key authentication and scopes, transaction orchestration, durable event ingestion, atomic quota operations, reservations, subscription lifecycle, adjustment workflow, and invoice finalization are outside this release. These are required before the project can be used as a production billing service.

A usage reservation is the integration boundary for work that may fail after admission: reserve capacity, perform the external operation, then commit or cancel. No billing service can make that external operation atomic with its own database transaction.
