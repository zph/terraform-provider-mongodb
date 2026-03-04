# Resource Capability Gating Requirements

## Overview

This document defines EARS requirements for the allowlist-based capability
gating mechanism that controls which Terraform resources are available at
runtime. Resources are classified as either mature (always available) or
experimental (require explicit opt-in via environment variable).

**System Name:** Resource Registry
**File:** `mongodb/resource_registry.go`

## Classification

**GATE-001** (Ubiquitous): Every resource in the provider SHALL be classified
as either `mature` or `experimental` in the resource registry.

**GATE-002** (Ubiquitous): The following resources SHALL be classified as
mature: `mongodb_db_user`, `mongodb_db_role`, `mongodb_original_user`.

**GATE-003** (Ubiquitous): The following resources SHALL be classified as
experimental: `mongodb_shard_config`, `mongodb_shard`.

## Default Behavior

**GATE-004** (Ubiquitous): The provider SHALL register all mature resources
unconditionally, regardless of environment variable state.

**GATE-005** (Unwanted Behaviour): IF no `TERRAFORM_PROVIDER_MONGODB_ENABLE`
environment variable is set, THEN the provider SHALL NOT register any
experimental resources.

## Opt-In

**GATE-006** (Event Driven): WHEN the `TERRAFORM_PROVIDER_MONGODB_ENABLE`
environment variable is set to a comma-separated list of resource names,
the provider SHALL register those experimental resources in addition to
all mature resources.

**GATE-007** (Ubiquitous): The provider SHALL trim whitespace from each
resource name in the comma-separated enable list.

**GATE-008** (Unwanted Behaviour): IF the enable list contains a resource
name that does not exist in the registry, THEN the provider SHALL ignore
the unknown name without error.

## Immutability

**GATE-009** (Ubiquitous): Mature resources SHALL NOT be blockable or
removable via the enable environment variable.

**GATE-010** (Ubiquitous): New resources added to the provider SHALL default
to `experimental` classification until explicitly promoted to `mature` in
the source code.
