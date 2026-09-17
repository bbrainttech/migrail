GO ?= go
BIN := bin/migrail

export CGO_ENABLED := 1

PG_VERSIONS ?= 12,13,14,15,16,17,18

ifeq ($(shell uname -s),Darwin)
ifeq ($(origin SDKROOT),undefined)
MACOS_SDKROOT := $(shell tools/macos-sdkroot.sh)
ifneq ($(MACOS_SDKROOT),)
export SDKROOT := $(MACOS_SDKROOT)
export MACOSX_DEPLOYMENT_TARGET := $(shell tools/macos-sdkroot.sh --version)
export CGO_CFLAGS := -O2 -g -mmacosx-version-min=$(MACOSX_DEPLOYMENT_TARGET)
export CGO_LDFLAGS := -mmacosx-version-min=$(MACOSX_DEPLOYMENT_TARGET)
endif
endif
endif

.PHONY: build test lint fmt tidy golden-update lockverify snapshot clean

build:
	$(GO) build -trimpath -o $(BIN) ./cmd/migrail

test:
	$(GO) test -race ./...

golden-update:
	UPDATE_GOLDEN=1 $(GO) test ./...

lockverify:
	$(GO) run ./tools/lockverify -versions $(PG_VERSIONS)

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
