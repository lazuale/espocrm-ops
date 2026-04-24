SHELL := bash
APP := espops
BIN := bin/$(APP)
HEALTH_TOOLS_BIN ?= $(HOME)/go/bin
STATICCHECK_VERSION ?= v0.7.0
GOLANGCI_LINT_VERSION ?= v2.11.4
INTEGRATION_PKGS := ./internal/runtime

.PHONY: build test test-race test-readonly integration integration-preflight ci staticcheck lint fmt vet mod-verify mod-clean-check coverage clean install-health-tools install-ci-health-tools

build:
	mkdir -p bin
	go build -o $(BIN) ./cmd/espops

test:
	go test ./...

test-race:
	go test -race ./...

test-readonly:
	go test -mod=readonly ./...

integration-preflight:
	@docker info >/dev/null
	@docker compose version >/dev/null

integration: integration-preflight
	go test -count=1 -p 1 -tags=integration $(INTEGRATION_PKGS)

mod-verify:
	go mod verify

mod-clean-check:
	git diff --exit-code -- go.mod go.sum

staticcheck:
	staticcheck ./...

lint:
	golangci-lint run --no-config ./...

ci: build mod-verify test-readonly test-race vet staticcheck lint integration mod-clean-check

fmt:
	go fmt ./...

vet:
	go vet ./...

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

install-health-tools:
	mkdir -p "$(HEALTH_TOOLS_BIN)"
	GOBIN="$(HEALTH_TOOLS_BIN)" go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	GOBIN="$(HEALTH_TOOLS_BIN)" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

install-ci-health-tools: install-health-tools

clean:
	rm -rf bin coverage.out
