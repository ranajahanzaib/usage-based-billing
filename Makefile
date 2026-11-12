.PHONY: test build run fmt vet
fmt:
	gofmt -w ./cmd ./internal
test:
	go test ./...
vet:
	go vet ./...
build:
	go build ./cmd/server
run:
	DEMO_MODE=1 go run ./cmd/server
