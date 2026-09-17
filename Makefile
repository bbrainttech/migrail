GO ?= go
BIN := bin/migrail

export CGO_ENABLED := 1

.PHONY: build test lint fmt tidy snapshot clean

build:
	$(GO) build -trimpath -o $(BIN) ./cmd/migrail

test:
	$(GO) test -race ./...

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
