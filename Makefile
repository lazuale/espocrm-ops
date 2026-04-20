SHELL := bash
APP := espops
BIN := bin/$(APP)
HEALTH_TOOLS_BIN ?= $(HOME)/go/bin
STATICCHECK_VERSION ?= v0.7.0
GOLANGCI_LINT_VERSION ?= v2.11.4

.PHONY: build test test-race integration ci staticcheck lint fmt vet coverage clean install-health-tools install-ci-health-tools

build:
	mkdir -p bin
	go build -o $(BIN) ./cmd/espops

test:
	go test ./...

test-race:
	go test -race ./...

integration:
	go test ./... -run Integration

staticcheck:
	staticcheck ./...

lint:
	golangci-lint run --no-config ./...

ci: build test test-race integration staticcheck lint

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
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(HEALTH_TOOLS_BIN)" $(GOLANGCI_LINT_VERSION)

install-ci-health-tools: install-health-tools

clean:
	rm -rf bin coverage.out
