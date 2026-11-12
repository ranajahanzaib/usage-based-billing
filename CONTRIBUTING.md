# Contributing

Contributions are welcome. Please open an issue for substantial changes so the intended behavior can be discussed before implementation.

## Development setup

The calculation library and local demo require Go 1.27 or newer. PostgreSQL is not required for the current demo.

```sh
git clone https://github.com/ranajahanzaib/usage-based-billing.git
cd usage-based-billing
go test ./...
go vet ./...
go build ./cmd/server
```

To run the local demonstration server:

```sh
DEMO_MODE=1 go run ./cmd/server
```

The demo binds to loopback by default and does not persist data. Do not use it with real customer data or expose it to a network.

## Pull requests

- Keep changes focused and explain the behavior they add or change.
- Add or update tests for behavioral changes.
- Run `gofmt`, `go test ./...`, `go test -race ./...`, and `go vet ./...` when Go code changes.
- Update the README or relevant documentation when commands, API behavior, billing semantics, or limitations change.
- Never include credentials, customer data, or generated build artifacts.
- Describe any checks that could not be run and why.

The project is licensed under MIT. Contributions are submitted under the same license.
