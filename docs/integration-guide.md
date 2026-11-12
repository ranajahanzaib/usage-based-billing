# Integration guide

## Current interfaces

The Go calculation entry point is `billing.PreviewInvoice` in `internal/billing`. It accepts plan values, effective subscription segments, counter events, storage history, a UTC billing interval, and an explicit `asOf` timestamp. Monetary prices and allowances are decimal strings. See `internal/billing/engine_test.go` for executable examples.

The local demo provides `GET /api/customers`, `GET /api/preview`, and `POST /api/simulate`. The simulation endpoint recalculates a preview with a bounded counter quantity and discards the added quantity after responding. The route definitions are in [openapi.yaml](openapi.yaml). These routes are demonstration endpoints, not a customer ingestion API.

## Production integration pattern

A production metering API should assign each logical usage event a stable source and event identity. Retries after a timeout must reuse the same identity and payload; reusing an identity with different content should return a conflict. Persist the canonical event ledger and enforce uniqueness in the database transaction that updates any derived quota state.

For work that can fail after quota admission, reserve capacity, execute the external operation, and then commit or cancel the reservation. If the billing request times out after a commit, retry with the original identity. A canceled or expired reservation must release capacity without creating a usage event. Direct atomic consumption is appropriate when the metered action has already succeeded or cannot be rolled back.

The calculation library and demo do not implement these production workflows. The billing service also cannot atomically commit work in another application's database. If the service is unavailable, each integrating application must choose and document whether its product fails open or closed.
