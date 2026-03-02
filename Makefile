ifndef VERBOSE
	MAKEFLAGS += --no-print-directory
endif

default: help

.PHONY: help build install re-install lint test test-unit test-plan test-shard-plan run

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
TERRAFORM_PLUGINS_DIRECTORY=~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build the provider binary
	go build -o terraform-provider-${NAME}

install: ## Build and install provider to Terraform plugins directory
	mkdir -p ${TERRAFORM_PLUGINS_DIRECTORY}
	go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd examples && rm -rf .terraform
	cd examples && make init

re-install: ## Clean reinstall of the provider
	rm -f examples/.terraform.lock.hcl
	rm -f ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	go build -o ${TERRAFORM_PLUGINS_DIRECTORY}/terraform-provider-${NAME}
	cd examples && rm -rf .terraform
	cd examples && make init

lint: ## Run golangci-lint
	golangci-lint run

test: test-unit test-plan test-shard-plan ## Run all tests

test-unit: ## Run Go unit tests
	go test ./...

test-plan: re-install ## Build provider and run terraform plan against examples
	cd examples && terraform plan

test-shard-plan: ## Build provider and run terraform plan for shard_config example
	cd examples/modules/shard_config/basic && make build

run: install ## Alias for install
