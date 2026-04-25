SHELL := bash
APP := espops
BIN := bin/$(APP)
HEALTH_TOOLS_BIN ?= $(HOME)/go/bin
STATICCHECK_VERSION ?= v0.7.0
GOLANGCI_LINT_VERSION ?= v2.11.4
INTEGRATION_PKGS := ./internal/runtime
INTEGRATION_IMAGES := mariadb:11.4 alpine:3.20

.PHONY: build test test-race test-readonly integration integration-preflight pull-images ci-fast ci-integration ci staticcheck lint fmt vet mod-verify mod-clean-check coverage clean install-health-tools install-ci-health-tools

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
	@echo "Checking Docker daemon..."
	@docker info >/dev/null
	@echo "Checking Docker Compose plugin..."
	@docker compose version >/dev/null

pull-images: integration-preflight
	@echo "Pulling required integration images..."
	@set -euo pipefail; \
	for image in $(INTEGRATION_IMAGES); do \
		echo "docker pull $$image"; \
		if output="$$(docker pull "$$image" 2>&1)"; then \
			printf '%s\n' "$$output"; \
			continue; \
		fi; \
		printf '%s\n' "$$output" >&2; \
		case "$$output" in \
			*'i/o timeout'*|*'TLS handshake timeout'*|*'dial tcp'*|*'no such host'*|*'temporary failure in name resolution'*|*'Client.Timeout exceeded'*) \
				echo "Image pull failed for $$image: registry/network unavailable while reaching Docker Hub." >&2 ;; \
			*'pull access denied'*|*'requested access to the resource is denied'*|*'unauthorized'*|*'toomanyrequests'*|*'rate limit'*) \
				echo "Image pull failed for $$image: auth failure or Docker Hub rate-limit." >&2 ;; \
			*'manifest unknown'*|*'not found'*) \
				echo "Image pull failed for $$image: requested image does not exist in the registry." >&2 ;; \
			*) \
				echo "Image pull failed for $$image: docker pull did not succeed." >&2 ;; \
		esac; \
		echo "Required integration images are not available locally, so Docker integration is not proven." >&2; \
		exit 1; \
	done

integration: pull-images
	@echo "Running real Docker integration tests..."
	go test -count=1 -p 1 -tags=integration $(INTEGRATION_PKGS)

mod-verify:
	go mod verify

mod-clean-check:
	git diff --exit-code -- go.mod go.sum

staticcheck:
	staticcheck ./...

lint:
	golangci-lint run --no-config ./...

ci-fast: build mod-verify test-readonly test-race vet staticcheck lint mod-clean-check

ci-integration: pull-images integration

ci: ci-fast ci-integration

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
