- [x] Setup publishing to terraform registry
- [x] Add section for why the fork
- [x] Strip out all Atlas and leave snarky mention about not supporting fear based extoration in
- [x] Same w/ documentDB for different reasons
readme for why we don't support atlas
- [x] Include a cdktf or cdk-terrain module to make using this on clusters convenient and compact
- [x] update testing harness
- [x] Update integration tests to be wrapped in automatic containers not some manual process of
- [x] Expand to support configuration for replicaset behavior like chainable, millisecond configs
for election/failover/heartbeats and profiler if not already present and diff for what we're missing
setup
- [x] Remove current generator format?
- [x] Add allowlist-based capability gating: Implement a registry mapping each resource to mature/experimental status. Only user management (mongodb_db_user, mongodb_db_role, mongodb_original_user) should be on the mature allowlist initially. All other resources (mongodb_shard_config, etc.) are blocked by default. Use a separate env var TERRAFORM_PROVIDER_MONGODB_ENABLE=<comma-separated-resources> to opt in specific experimental resources without modifying source code (e.g. TERRAFORM_PROVIDER_MONGODB_ENABLE=mongodb_shard_config). New resources default to blocked until explicitly promoted to the mature allowlist in code or opted in via the env var at runtime.
