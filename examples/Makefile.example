ROOT_DIR := $(shell git rev-parse --show-toplevel)
include $(ROOT_DIR)/Makefile

TF_PLUGINS_DIR=${HOME}/.terraform.d/plugins

init: ## Initialize terraform with local plugin directory
	terraform init \
	-plugin-dir=$(TF_PLUGINS_DIR)

apply: ## Run terraform apply
	terraform apply

plan: ## Run terraform plan
	terraform plan

destroy: ## Run terraform destroy
	terraform destroy
