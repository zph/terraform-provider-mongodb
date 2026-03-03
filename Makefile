ifndef VERBOSE
	MAKEFLAGS += --no-print-directory
endif

# Root directory of the provider (works when included from subdirectories)
PROVIDER_ROOT := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

default: help

.PHONY: help setup dev-overrides build install re-install lint prek prek-install test test-all test-unit test-integration test-sharded-integration test-golden test-golden-update test-plan test-shard-plan run cdktn-build cdktn-test cdktn-test-golden

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
VERSION=9.9.9
## on linux base os
TERRAFORM_PLUGINS_DIRECTORY=$(HOME)/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}
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

build: ## Build the provider binary
	cd $(PROVIDER_ROOT) && go build -o terraform-provider-${NAME}

install: ## Build and install provider to Terraform plugins directory
	mkdir -p ${TERRAFORM_PLUGINS_DIRECTORY}
	cd $(PROVIDER_ROOT) && go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}

re-install: ## Clean reinstall of the provider
	rm -f $(PROVIDER_ROOT)/examples/.terraform.lock.hcl
	rm -f ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd $(PROVIDER_ROOT) && go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd $(PROVIDER_ROOT)/examples && rm -rf .terraform
	cd $(PROVIDER_ROOT)/examples && make init

lint: ## Run all prek hooks on all files
	cd $(PROVIDER_ROOT) && prek run --all-files

prek: lint ## Alias for lint

prek-install: ## Install prek git hooks (pre-commit + pre-push)
	prek install -t pre-commit -t pre-push

test: test-unit cdktn-test test-plan test-shard-plan ## Run all tests (unit + cdktn + plan)

test-all: test-unit cdktn-test test-integration test-sharded-integration test-golden test-plan test-shard-plan ## Run every test suite (unit, cdktn, integration, sharded, golden, plan)

test-unit: ## Run Go unit tests
	cd $(PROVIDER_ROOT) && go test ./...

test-integration: ## Run integration tests (requires Docker)
	cd $(PROVIDER_ROOT) && go test -tags integration -v -timeout 300s ./mongodb/

test-plan: re-install ## Build provider and run terraform plan against examples
	cd $(PROVIDER_ROOT)/examples && terraform plan

test-shard-plan: export TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_config,mongodb_shard
test-shard-plan: re-install ## Build provider and run terraform plan for shard_config example
	cd $(PROVIDER_ROOT)/examples/modules/shard_config/basic && terraform plan

run: install ## Alias for install

test-sharded-integration: ## Run sharded cluster integration tests (requires Docker, slower)
	cd $(PROVIDER_ROOT) && go test -tags integration -run TestShardedIntegration -v -timeout 600s ./mongodb/

test-golden: ## Run golden file tests against MongoDB container
	cd $(PROVIDER_ROOT) && go test -tags integration -run TestGolden -v -timeout 300s ./mongodb/

test-golden-update: ## Regenerate golden files
	cd $(PROVIDER_ROOT) && UPDATE_GOLDEN=1 go test -tags integration -run TestGolden -v -timeout 300s ./mongodb/

cdktn-build: ## Build the CDKTN construct library
	cd $(PROVIDER_ROOT)/cdktn && go build ./...

cdktn-test: ## Run CDKTN construct library tests
	cd $(PROVIDER_ROOT)/cdktn && go test ./...

cdktn-test-golden: ## Update CDKTN golden files
	cd $(PROVIDER_ROOT)/cdktn && UPDATE_GOLDEN=1 go test ./...
