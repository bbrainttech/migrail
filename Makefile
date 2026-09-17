GO ?= go
BIN := bin/migrail

export CGO_ENABLED := 1

.PHONY: build test lint fmt tidy golden-update snapshot clean

build:
	$(GO) build -trimpath -o $(BIN) ./cmd/migrail

test:
	$(GO) test -race ./...

golden-update:
	UPDATE_GOLDEN=1 $(GO) test ./internal/cli/...

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

tidy:
	$(GO) mod tidy

snapshot:
	goreleaser build --snapshot --clean --single-target

clean:
	rm -rf bin dist
