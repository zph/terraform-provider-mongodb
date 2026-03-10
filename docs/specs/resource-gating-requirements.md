# Resource Capability Gating Requirements

## Overview

This document defines EARS requirements for the allowlist-based capability
gating mechanism that controls which Terraform resources are available at
runtime. Resources are classified as either mature (always available) or
experimental (require explicit opt-in).

All resources are registered in the provider schema unconditionally.
Experimental resources are gated at plan time via `CustomizeDiff` and
require opt-in through the provider's `features_enabled` argument or the
`TERRAFORM_PROVIDER_MONGODB_ENABLE` environment variable.

**System Name:** Resource Registry
**Files:** `mongodb/resource_registry.go`, `mongodb/provider.go`

## Classification

**GATE-001** (Ubiquitous): Every resource in the provider SHALL be classified
as either `mature` or `experimental` in the resource registry.

**GATE-002** (Ubiquitous): The following resources SHALL be classified as
mature: `mongodb_db_user`, `mongodb_db_role`, `mongodb_original_user`.

**GATE-003** (Ubiquitous): The following resources SHALL be classified as
experimental: `mongodb_shard_config`, `mongodb_shard`, `mongodb_profiler`,
`mongodb_server_parameter`, `mongodb_balancer_config`,
`mongodb_collection_balancing`, `mongodb_feature_compatibility_version`.

## Registration

**GATE-004** (Ubiquitous): The provider SHALL register all resources (both
mature and experimental) unconditionally in `ResourcesMap`, so that HCL
references to experimental resources are syntactically valid regardless
of feature flag state.

## Plan-Time Gating

**GATE-005** (Event Driven): WHEN an experimental resource is included in
a plan AND the resource name is NOT in the merged enable set (from
`features_enabled` + `TERRAFORM_PROVIDER_MONGODB_ENABLE`), the
`CustomizeDiff` SHALL return an error blocking the plan.

**GATE-006** (Event Driven): WHEN the provider `features_enabled` argument
is set to a list of resource names, those names SHALL be merged with the
`TERRAFORM_PROVIDER_MONGODB_ENABLE` environment variable into a single
enable set stored in `MongoDatabaseConfiguration.FeaturesEnabled`.

**GATE-007** (Ubiquitous): The provider SHALL trim whitespace from each
resource name in the comma-separated environment variable.

**GATE-008** (Ubiquitous): The `features_enabled` argument SHALL validate
that each entry is a recognized experimental resource name.

**GATE-009** (Event Driven): WHEN the provider metadata is nil (e.g. during
`terraform validate`), the `requireFeature` check SHALL pass without error.

## Immutability

**GATE-010** (Ubiquitous): Mature resources SHALL NOT be blockable or
removable via the enable mechanism.

**GATE-011** (Ubiquitous): New resources added to the provider SHALL default
to `experimental` classification until explicitly promoted to `mature` in
the source code.
