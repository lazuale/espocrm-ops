SHELL := bash
PYTHON ?= python3
APP := espops
BIN := bin/$(APP)
HEALTH_TOOLS_BIN ?= $(HOME)/go/bin
GOVULNCHECK_VERSION ?= v1.2.0
STATICCHECK_VERSION ?= v0.7.0
GOLANGCI_LINT_VERSION ?= v2.11.4
FAST_GATE_COMPONENTS := test vet ai-shell-json-smoke bashcheck shellcheck
SHELL_SCRIPTS := scripts/espo.sh scripts/doctor.sh scripts/backup.sh scripts/restore.sh scripts/migrate.sh scripts/regression-test.sh scripts/lib/common.sh

.PHONY: ai-validate ai-refresh ai-check ai-shell-json-smoke build test test-cli test-golden fmt vet clean integration ci check-fast check-fast-components check-full regression bashcheck shellcheck vulncheck staticcheck lint coverage install-health-tools install-ci-health-tools

ai-validate:
	$(PYTHON) AI/generators/validate_specs.py

ai-refresh: ai-validate
	$(PYTHON) AI/generators/contract_diff.py --write-baseline
	$(PYTHON) AI/generators/json_fixture_contract_diff.py --write-baseline

ai-check: ai-validate
	$(PYTHON) AI/generators/ast_arch_guard.py
	$(PYTHON) AI/generators/contract_diff.py --check
	$(PYTHON) AI/generators/json_fixture_contract_diff.py --check
	$(PYTHON) AI/generators/runner.py shell-guard
	$(PYTHON) AI/generators/runner.py docs-sync
	$(PYTHON) AI/generators/runner.py test-sync
	$(PYTHON) AI/generators/runner.py package-guard

ai-shell-json-smoke: build
	tmp_root="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp_root"' EXIT; \
	env_dev="$$tmp_root/.env.dev"; \
	runtime_dev="$$tmp_root/runtime/dev"; \
	backup_dev="$$tmp_root/backups/dev"; \
	mkdir -p "$$runtime_dev/db" "$$runtime_dev/espo" "$$backup_dev" "$$tmp_root/out"; \
	cp ops/env/.env.dev.example "$$env_dev"; \
	chmod 600 "$$env_dev"; \
	source scripts/lib/common.sh; \
	set_env_value "$$env_dev" DB_STORAGE_DIR "$$runtime_dev/db"; \
	set_env_value "$$env_dev" ESPO_STORAGE_DIR "$$runtime_dev/espo"; \
	set_env_value "$$env_dev" BACKUP_ROOT "$$backup_dev"; \
	set_env_value "$$env_dev" DB_ROOT_PASSWORD "dev-root-password"; \
	set_env_value "$$env_dev" DB_PASSWORD "dev-db-password"; \
	set_env_value "$$env_dev" ADMIN_PASSWORD "dev-admin-password"; \
	ENV_FILE="$$env_dev" bash scripts/doctor.sh dev --json > "$$tmp_root/out/doctor-dev.json" || true; \
	$(PYTHON) AI/generators/json_fixture_contract_diff.py --parse-files \
		"$$tmp_root/out/doctor-dev.json"

check-fast-components: $(FAST_GATE_COMPONENTS)

check-fast: ai-check check-fast-components

check-full:
	$(MAKE) ai-refresh
	git diff --exit-code -- AI/compiled
	$(MAKE) ai-check
	$(MAKE) check-fast-components
	go test -race ./...
	$(MAKE) staticcheck
	$(MAKE) lint
	$(MAKE) coverage
	$(MAKE) integration
	$(MAKE) regression

ci: check-fast

build:
	mkdir -p bin
	go build -o $(BIN) ./cmd/espops

test:
	go test ./...

test-cli:
	go test ./internal/cli/...

test-golden:
	go test ./internal/cli/... -run Golden

integration:
	go test ./... -run Integration

regression:
	bash scripts/regression-test.sh

vulncheck:
	govulncheck ./...

staticcheck:
	staticcheck ./...

lint:
	golangci-lint run --no-config ./...

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

install-health-tools:
	mkdir -p "$(HEALTH_TOOLS_BIN)"
	GOBIN="$(HEALTH_TOOLS_BIN)" go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	GOBIN="$(HEALTH_TOOLS_BIN)" go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(HEALTH_TOOLS_BIN)" $(GOLANGCI_LINT_VERSION)

install-ci-health-tools:
	mkdir -p "$(HEALTH_TOOLS_BIN)"
	GOBIN="$(HEALTH_TOOLS_BIN)" go install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(HEALTH_TOOLS_BIN)" $(GOLANGCI_LINT_VERSION)

bashcheck:
	bash -n $(SHELL_SCRIPTS)

shellcheck:
	shellcheck $(SHELL_SCRIPTS)

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin
