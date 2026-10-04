GO      ?= go
VERSION ?= dev
BIN     ?= bin/keepalive

.PHONY: check fmt-check vet test build docs snapshot release

## check: formatting, vet for every target OS, and the race-enabled test suite
check: fmt-check vet test

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet:
	GOOS=darwin $(GO) vet ./...
	GOOS=linux $(GO) vet ./...
	GOOS=windows $(GO) vet ./...

test:
	$(GO) test -race ./...

build:
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/keepalive

## docs: man pages in man/ and shell completions in docs/completions/
docs:
	$(GO) run ./cmd/gen-docs

## snapshot: every release artifact in dist/; the macOS binary is signed but
## not notarized, and nothing is published (KEEPALIVE_SIGN=0 skips signing)
snapshot:
	KEEPALIVE_NOTARIZE=0 goreleaser release --snapshot --clean

## release: guarded, interactive release of the tagged HEAD; see RELEASING.md
release:
	./scripts/release.sh
