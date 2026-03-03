ifndef VERBOSE
	MAKEFLAGS += --no-print-directory
endif

default: help

.PHONY: help setup dev-overrides build install re-install lint prek prek-install test test-unit test-integration test-plan test-shard-plan run cdktn-build cdktn-test cdktn-test-golden

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
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

setup: dev-overrides ## Set up dev environment (hermit, git hooks, go deps, dev_overrides)
	.hermit/bin/hermit install
	prek install
	go mod download

dev-overrides: ## Configure Terraform dev_overrides for local provider builds
	@if [ -f "$(TERRAFORMRC)" ] && grep -q 'dev_overrides' "$(TERRAFORMRC)"; then \
		echo "dev_overrides already configured in $(TERRAFORMRC)"; \
	else \
		printf 'provider_installation {\n  dev_overrides {\n    "%s/%s" = "%s"\n  }\n  direct {}\n}\n' \
			"$(NAMESPACE)" "$(NAME)" "$(TERRAFORM_PLUGINS_DIRECTORY)" >> "$(TERRAFORMRC)"; \
		echo "Added dev_overrides to $(TERRAFORMRC)"; \
	fi

build: ## Build the provider binary
	go build -o terraform-provider-${NAME}

install: ## Build and install provider to Terraform plugins directory
	mkdir -p ${TERRAFORM_PLUGINS_DIRECTORY}
	go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}

re-install: ## Clean reinstall of the provider
	rm -f examples/.terraform.lock.hcl
	rm -f ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd examples && rm -rf .terraform
	cd examples && make init

lint: ## Run all prek hooks on all files
	prek run --all-files

prek: lint ## Alias for lint

prek-install: ## Install prek as git pre-commit hook
	prek install

test: test-unit cdktn-test test-plan test-shard-plan ## Run all tests (unit + cdktn + plan)

test-unit: ## Run Go unit tests
	go test ./...

test-integration: ## Run integration tests (requires Docker)
	go test -tags integration -v -timeout 300s ./mongodb/

test-plan: re-install ## Build provider and run terraform plan against examples
	cd examples && terraform plan

test-shard-plan: ## Build provider and run terraform plan for shard_config example
	cd examples/modules/shard_config/basic && make build

run: install ## Alias for install

cdktn-build: ## Build the CDKTN construct library
	cd cdktn && go build ./...

cdktn-test: ## Run CDKTN construct library tests
	cd cdktn && go test ./...

cdktn-test-golden: ## Update CDKTN golden files
	cd cdktn && UPDATE_GOLDEN=1 go test ./...
