BINARY_NAME=arara
MODULE=github.com/ararahq/cli
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-s -w -X $(MODULE)/internal/version.Version=$(VERSION) -X $(MODULE)/internal/version.Commit=$(COMMIT) -X $(MODULE)/internal/version.Date=$(DATE)"

GOLANGCI_LINT_VERSION=v1.64.5
GOPATH_BIN=$(shell go env GOPATH)/bin
GOLANGCI_LINT=$(GOPATH_BIN)/golangci-lint

.PHONY: build run clean test test-race vet fmt lint cover cover-html install install-system snapshot tools ci manpages

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/arara/

run:
	go run $(LDFLAGS) ./cmd/arara/ $(ARGS)

install:
	go install $(LDFLAGS) ./cmd/arara/

# install-system: builds the binary with the LDFLAGS version stamping and
# copies it to /usr/local/bin/arara so the user can just type `arara` from
# anywhere in any terminal. Requires sudo because /usr/local/bin is
# system-owned on most setups.
install-system: build
	@echo "Installing arara to /usr/local/bin (requires sudo)..."
	@sudo install -m 0755 ./bin/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "✔ Installed. Try: arara --version"

test:
	go test -count=1 ./...

test-race:
	go test -race -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --timeout=5m

$(GOLANGCI_LINT):
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH_BIN) $(GOLANGCI_LINT_VERSION)

cover:
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | tail -1

cover-html: cover
	go tool cover -html=coverage.out

ci: vet lint test-race cover

clean:
	rm -rf bin/ dist/ coverage.out

snapshot:
	goreleaser build --snapshot --clean

manpages: build
	@./bin/$(BINARY_NAME) generate-manpages build/man

tools:
	go install golang.org/x/vuln/cmd/govulncheck@latest
