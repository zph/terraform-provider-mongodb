ifndef VERBOSE
	MAKEFLAGS += --no-print-directory
endif

# Root directory of the provider (works when included from subdirectories)
PROVIDER_ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

default: help

# Supported MongoDB versions for integration test matrix
MONGO_VERSIONS := 3.6 7

.PHONY: help setup dev-overrides build install re-install lint lint-noforceenew prek prek-install test test-all test-unit test-integration test-sharded-integration test-golden test-golden-update test-plan test-shard-plan test-integration-matrix test-integration-all test-ci run cdktn-build cdktn-test cdktn-test-golden tag release

OS_ARCH=linux_amd64
#
# Set correct OS_ARCH on Mac
UNAME := $(shell uname -s)
ifeq ($(UNAME),Darwin)
	HW := $(shell uname -m)
	ifeq ($(HW),arm64)
		ARCH=$(HW)
	else
		ARCH=amd64
	endif
	OS_ARCH=darwin_$(ARCH)
endif

HOSTNAME=registry.terraform.io
NAMESPACE=zph
NAME=mongodb
VERSION=$(shell cat $(PROVIDER_ROOT)/VERSION | tr -d '[:space:]')
# Local dev builds always use 9.9.9 so the binary lands where dev_overrides expects.
# Actual releases use VERSION (via tag/release targets).
DEV_VERSION=9.9.9
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
LDFLAGS=-s -w -X main.version=$(DEV_VERSION) -X main.commit=$(COMMIT)
## on linux base os
TERRAFORM_PLUGINS_DIRECTORY=$(HOME)/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${DEV_VERSION}/${OS_ARCH}
TERRAFORMRC=$(HOME)/.terraformrc

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

setup: dev-overrides ## Set up dev environment (hermit, git hooks, go deps, dev_overrides)
	cd $(PROVIDER_ROOT) && .hermit/bin/hermit install
	prek install -t pre-commit -t pre-push
	cd $(PROVIDER_ROOT) && go mod download

dev-overrides: ## Configure Terraform dev_overrides for local provider builds
	@if [ -f "$(TERRAFORMRC)" ] && grep -q 'dev_overrides' "$(TERRAFORMRC)"; then \
		echo "dev_overrides already configured in $(TERRAFORMRC)"; \
	else \
		printf 'provider_installation {\n  dev_overrides {\n    "%s/%s" = "%s"\n  }\n  direct {}\n}\n' \
			"$(NAMESPACE)" "$(NAME)" "$(TERRAFORM_PLUGINS_DIRECTORY)" >> "$(TERRAFORMRC)"; \
		echo "Added dev_overrides to $(TERRAFORMRC)"; \
	fi

build: ## Build the provider binary and install to plugins directory
	mkdir -p ${TERRAFORM_PLUGINS_DIRECTORY}
	cd $(PROVIDER_ROOT) && go build -ldflags "$(LDFLAGS)" -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}

install: ## Build and install provider to Terraform plugins directory
	mkdir -p ${TERRAFORM_PLUGINS_DIRECTORY}
	cd $(PROVIDER_ROOT) && go build -ldflags "$(LDFLAGS)" -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}

re-install: ## Clean reinstall of the provider
	rm -f $(PROVIDER_ROOT)/examples/.terraform.lock.hcl
	rm -f ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd $(PROVIDER_ROOT) && go build -ldflags "$(LDFLAGS)" -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd $(PROVIDER_ROOT)/examples && rm -rf .terraform
	cd $(PROVIDER_ROOT)/examples && make init

NOFORCEENEW_BIN := $(PROVIDER_ROOT)/.hermit/.cache/noforceenew

lint: lint-noforceenew ## Run all linters and prek hooks on all files
	cd $(PROVIDER_ROOT) && prek run --all-files

lint-noforceenew: ## Ban ForceNew: true in schemas (DANGER-010)
	@cd $(PROVIDER_ROOT) && go build -o $(NOFORCEENEW_BIN) ./linters/noforceenew/cmd/noforceenew
	cd $(PROVIDER_ROOT) && $(NOFORCEENEW_BIN) -allow=resource_shard.go:shard_name ./mongodb/...

prek: lint ## Alias for lint

prek-install: ## Install prek git hooks (pre-commit + pre-push)
	prek install -t pre-commit -t pre-push

test: test-unit cdktn-test test-plan test-shard-plan ## Run all tests (unit + cdktn + plan)

test-all: test-unit cdktn-test test-integration test-sharded-integration test-golden test-plan test-shard-plan ## Run every test suite (unit, cdktn, integration, sharded, golden, plan)

test-ci: test-unit cdktn-test test-integration-matrix test-sharded-integration test-golden ## Unit + cdktn + integration matrix + sharded + golden tests

test-unit: ## Run Go unit tests
	cd $(PROVIDER_ROOT) && go test ./...

test-integration: ## Run integration tests excluding golden (requires Docker; override image with MONGO_TEST_IMAGE)
	cd $(PROVIDER_ROOT) && go test -tags integration -run 'TestIntegration_|TestShardedIntegration_' -v -timeout 300s ./mongodb/

test-integration-matrix: ## Run integration tests against all supported MongoDB versions
	@for v in $(MONGO_VERSIONS); do \
		echo "=== mongo:$$v ==="; \
		MONGO_TEST_IMAGE="mongo:$$v" $(MAKE) test-integration || exit 1; \
	done

test-integration-all: ## Run all integration tests including golden (requires Docker)
	cd $(PROVIDER_ROOT) && go test -tags integration -v -timeout 300s ./mongodb/

test-plan: re-install ## Build provider and run terraform plan against examples
	cd $(PROVIDER_ROOT)/examples && terraform plan

test-shard-plan: export TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_config,mongodb_shard
test-shard-plan: re-install ## Build provider and run terraform plan for shard_config example
	cd $(PROVIDER_ROOT)/examples/modules/shard_config/basic && rm -rf .terraform .terraform.lock.hcl && make init && terraform plan

run: install ## Alias for install

test-sharded-integration: ## Run sharded cluster integration tests (requires Docker, slower)
	cd $(PROVIDER_ROOT) && go test -tags integration -run 'TestShardedIntegration|TestGolden_Shard' -v -timeout 600s ./mongodb/

test-golden: ## Run golden file tests against MongoDB container
	cd $(PROVIDER_ROOT) && go test -tags integration -run TestGolden -v -timeout 600s ./mongodb/

test-golden-update: ## Regenerate golden files
	cd $(PROVIDER_ROOT) && UPDATE_GOLDEN=1 go test -tags integration -run TestGolden -v -timeout 600s ./mongodb/

cdktn-build: ## Build the CDKTN construct library
	cd $(PROVIDER_ROOT)/cdktn && go build ./...

cdktn-test: ## Run CDKTN construct library tests
	cd $(PROVIDER_ROOT)/cdktn && go test ./...

cdktn-test-golden: ## Update CDKTN golden files
	cd $(PROVIDER_ROOT)/cdktn && UPDATE_GOLDEN=1 go test ./...

tag: ## Create a git tag from the VERSION file (v-prefixed)
	@if git rev-parse "v$(VERSION)" >/dev/null 2>&1; then \
		echo "Tag v$(VERSION) already exists"; exit 1; \
	fi
	git tag -a "v$(VERSION)" -m "Release v$(VERSION)"
	@echo "Tagged v$(VERSION). Run 'git push origin v$(VERSION)' to push."

release: ## Bump VERSION (strip -dev), commit, tag, and push to trigger release
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is dirty. Commit or stash changes first."; exit 1; \
	fi
	@CURRENT=$$(cat $(PROVIDER_ROOT)/VERSION | tr -d '[:space:]'); \
	RELEASE=$${CURRENT%-dev}; \
	if [ "$$CURRENT" = "$$RELEASE" ]; then \
		echo "VERSION ($$CURRENT) has no -dev suffix. Bump manually or pass BUMP=patch|minor|major."; exit 1; \
	fi; \
	echo "$$RELEASE" > $(PROVIDER_ROOT)/VERSION; \
	git add $(PROVIDER_ROOT)/VERSION; \
	git commit -m "Release v$$RELEASE"; \
	git tag -a "v$$RELEASE" -m "Release v$$RELEASE"; \
	git push && git push origin "v$$RELEASE"; \
	NEXT_PATCH=$$(echo "$$RELEASE" | awk -F. '{printf "%s.%s.%s", $$1, $$2, $$3+1}'); \
	echo "$${NEXT_PATCH}-dev" > $(PROVIDER_ROOT)/VERSION; \
	git add $(PROVIDER_ROOT)/VERSION; \
	git commit -m "Begin v$${NEXT_PATCH}-dev"; \
	git push; \
	echo "Released v$$RELEASE and bumped to $${NEXT_PATCH}-dev"
