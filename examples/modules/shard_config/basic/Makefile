build:
	@cd ../../../.. && \
		make re-install && \
		cd examples/modules/shard_config/basic && \
		rm -f .terraform.lock* && terraform init && terraform plan

