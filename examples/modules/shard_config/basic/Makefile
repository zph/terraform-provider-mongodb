ROOT_DIR := $(shell git rev-parse --show-toplevel)
include $(ROOT_DIR)/Makefile

build: ## Build provider and plan this example
	@cd $(PROVIDER_ROOT) && \
		make re-install && \
		cd examples/modules/shard_config/basic && \
		rm -f .terraform.lock* && terraform init && terraform plan

apply: ## Run terraform apply
	terraform apply

plan: ## Run terraform plan
	terraform plan
