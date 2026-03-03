# Resource ID Format Requirements

## Overview

Resource IDs for `mongodb_db_user`, `mongodb_db_role`, and `mongodb_original_user` were previously stored as Base64-encoded strings of `database.name`. This encoding added unnecessary complexity for users during import operations with no technical benefit. This specification defines the plain text ID format and shared parsing/formatting helpers.

**System Name:** Terraform MongoDB Provider
**Version:** 1.0
**Last Updated:** 2026-03-03

## Requirements

### ID Format

**IDFORMAT-001:** Ubiquitous

**Requirement:**
The Terraform MongoDB Provider SHALL use plain text `database.name` format for all resource IDs across `mongodb_db_user`, `mongodb_db_role`, and `mongodb_original_user` resources.

**Rationale:**
Terraform IDs can be any string. Base64 encoding provided no security or technical benefit and forced users to manually encode values during import operations.

**Verification:**
Unit tests confirm that resource Create, Update, and Adopt operations set IDs in `database.name` format without Base64 encoding.

---

### ID Parsing

**IDFORMAT-002:** Event Driven

**Requirement:**
WHEN a resource ID is parsed, the Terraform MongoDB Provider SHALL split on the first `.` separator using `SplitN(id, ".", 2)` and return the name and database components.

**Rationale:**
Splitting on the first dot preserves names that themselves contain dots (e.g., `admin.my.dotted.role`), while still separating the database prefix.

**Verification:**
Unit tests confirm that `parseResourceId("admin.my.dotted.role")` returns `database="admin"` and `name="my.dotted.role"`.

---

**IDFORMAT-003:** Unwanted Behaviour

**Requirement:**
If a resource ID is missing a `.` separator or has an empty database or name component, then the Terraform MongoDB Provider SHALL return a descriptive error.

**Rationale:**
Malformed IDs must be rejected with clear error messages rather than silently producing incorrect behavior.

**Verification:**
Unit tests confirm that `parseResourceId("nodots")`, `parseResourceId(".name")`, and `parseResourceId("db.")` all return non-nil errors.

---

### ID Formatting

**IDFORMAT-004:** Event Driven

**Requirement:**
WHEN a resource ID is formatted, the Terraform MongoDB Provider SHALL return the concatenation `database + "." + name`.

**Rationale:**
A single shared helper ensures consistent ID construction across all resource types.

**Verification:**
Unit tests confirm that `formatResourceId("admin", "testuser")` returns `"admin.testuser"`.

---

### Shared Helpers

**IDFORMAT-005:** Ubiquitous

**Requirement:**
The Terraform MongoDB Provider SHALL use the shared `parseResourceId` and `formatResourceId` helpers in all resource types (`mongodb_db_user`, `mongodb_db_role`, `mongodb_original_user`) instead of per-resource parsing or inline ID construction.

**Rationale:**
Centralizing ID logic in `parse_id.go` eliminates duplicated code and ensures consistent behavior across all resources.

**Verification:**
Code review confirms that `resource_db_user.go`, `resource_db_role.go`, and `resource_original_user.go` delegate to `parseResourceId` and `formatResourceId` with no inline Base64 or string splitting.

---
