# Unit Test Suite Requirements

## Overview

This document specifies requirements for pure Go unit tests in the terraform-provider-mongodb project. These tests cover functions and methods that can be validated without a live MongoDB instance, targeting approximately 25-33% of the `mongodb/` package.

**System Name:** Unit Test Suite
**Version:** 1.0
**Last Updated:** 2026-03-02

## Requirements

### ID Parsing

All three resource types encode their Terraform state ID as base64("database.name"). The parse functions decode and split this, returning (name, database, error).

**Test files:** `mongodb/resource_db_user_test.go`, `mongodb/resource_db_role_test.go`, `mongodb/resource_shard_config_test.go`
**Source functions:** `resourceDatabaseUserParseId`, `resourceDatabaseRoleParseId`, `ResourceShardConfig.ParseId`

**TEST-001:** Event Driven

**Requirement:**
When `resourceDatabaseUserParseId` receives a valid base64-encoded ID in the format "database.username", the Unit Test Suite SHALL return the username as the first value and the database as the second value with a nil error.

**Rationale:**
The ID parsing is the inverse of ID creation and MUST correctly decompose the encoded ID to retrieve resources from MongoDB.

**Verification:**
Encode "admin.testuser" as base64, call `resourceDatabaseUserParseId`, assert returns ("testuser", "admin", nil).

---

**TEST-002:** Unwanted Behaviour

**Requirement:**
If `resourceDatabaseUserParseId` receives an invalid base64 string, then the Unit Test Suite SHALL return empty strings and a non-nil error.

**Rationale:**
Corrupted or tampered state IDs MUST produce a clear error rather than a panic or silent failure.

**Verification:**
Call with "not-valid-base64!@#", assert error is non-nil.

---

**TEST-003:** Unwanted Behaviour

**Requirement:**
If `resourceDatabaseUserParseId` receives a valid base64 string that does not contain a "." separator, then the Unit Test Suite SHALL return empty strings and a non-nil error.

**Rationale:**
A base64-decodable string without the expected format is still invalid as a resource ID.

**Verification:**
Encode "nodotshere" as base64, call function, assert error is non-nil.

---

**TEST-004:** Unwanted Behaviour

**Requirement:**
If `resourceDatabaseUserParseId` receives a valid base64 string where either the database or username portion is empty (e.g., ".username" or "database."), then the Unit Test Suite SHALL return empty strings and a non-nil error.

**Rationale:**
Both components of the ID are required for resource lookups.

**Verification:**
Encode ".username" and "database." as base64, call function for each, assert error is non-nil for both.

---

**TEST-005:** Event Driven

**Requirement:**
When `resourceDatabaseRoleParseId` receives a valid base64-encoded ID in the format "database.roleName", the Unit Test Suite SHALL return the roleName as the first value and the database as the second value with a nil error.

**Rationale:**
Role ID parsing follows the same contract as user ID parsing.

**Verification:**
Encode "admin.myRole" as base64, call function, assert returns ("myRole", "admin", nil).

---

**TEST-006:** Unwanted Behaviour

**Requirement:**
If `resourceDatabaseRoleParseId` receives invalid base64 or a decoded string missing the "." separator or containing empty parts, then the Unit Test Suite SHALL return empty strings and a non-nil error.

**Rationale:**
Same error contract as user ID parsing applies to role ID parsing.

**Verification:**
Repeat TEST-002, TEST-003, TEST-004 patterns against `resourceDatabaseRoleParseId`.

---

**TEST-007:** Event Driven

**Requirement:**
When `ResourceShardConfig.ParseId` receives a valid base64-encoded ID in the format "database.shardName", the Unit Test Suite SHALL return the shardName as the first value and the database as the second value with a nil error.

**Rationale:**
Shard config ID parsing follows the same contract as user and role ID parsing.

**Verification:**
Encode "admin.shard01" as base64, call method, assert returns ("shard01", "admin", nil).

---

**TEST-008:** Unwanted Behaviour

**Requirement:**
If `ResourceShardConfig.ParseId` receives invalid base64 or a decoded string missing the "." separator or containing empty parts, then the Unit Test Suite SHALL return empty strings and a non-nil error.

**Rationale:**
Same error contract as user and role ID parsing applies to shard config ID parsing.

**Verification:**
Repeat TEST-002, TEST-003, TEST-004 patterns against `ResourceShardConfig.ParseId`.

---

### URI Argument Builder

**Test file:** `mongodb/config_test.go`
**Source function:** `addArgs`

**TEST-009:** Event Driven

**Requirement:**
When `addArgs` receives an empty arguments string and a new argument, the Unit Test Suite SHALL return a string prefixed with "/?" followed by the new argument.

**Rationale:**
The first query parameter in a MongoDB URI MUST be preceded by "/?".

**Verification:**
Call `addArgs("", "ssl=true")`, assert returns "/?ssl=true".

---

**TEST-010:** Event Driven

**Requirement:**
When `addArgs` receives a non-empty arguments string and a new argument, the Unit Test Suite SHALL return the original arguments with "&" and the new argument appended.

**Rationale:**
Subsequent query parameters in a URI MUST be separated by "&".

**Verification:**
Call `addArgs("/?ssl=true", "replicaSet=rs0")`, assert returns "/?ssl=true&replicaSet=rs0".

---

### Proxy Dialer

**Test file:** `mongodb/config_test.go`
**Source function:** `proxyDialer`

**TEST-011:** Event Driven

**Requirement:**
When `proxyDialer` is called with a `ClientConfig` containing a valid SOCKS5 proxy URL, the Unit Test Suite SHALL return a non-nil ContextDialer and a nil error.

**Rationale:**
A valid proxy URL MUST produce a usable dialer.

**Verification:**
Create ClientConfig with `Proxy: "socks5://127.0.0.1:1080"`, call `proxyDialer`, assert dialer is non-nil and error is nil.

---

**TEST-012:** Unwanted Behaviour

**Requirement:**
If `proxyDialer` is called with a `ClientConfig` containing an invalid proxy URL, then the Unit Test Suite SHALL return a non-nil error.

**Rationale:**
Malformed proxy URLs MUST be rejected with a clear error.

**Verification:**
Create ClientConfig with `Proxy: "://not-a-url"`, call `proxyDialer`, assert error is non-nil.

---

**TEST-013:** Event Driven

**Requirement:**
When `proxyDialer` is called with a `ClientConfig` containing an empty Proxy field, the Unit Test Suite SHALL return the environment-based proxy dialer and a nil error.

**Rationale:**
Empty proxy config MUST fall back to environment variable proxy detection.

**Verification:**
Create ClientConfig with `Proxy: ""`, call `proxyDialer`, assert dialer is non-nil and error is nil.

---

### Type String Methods

**Test file:** `mongodb/config_test.go`
**Source methods:** `Role.String()`, `Privilege.String()`, `Resource.String()`

**TEST-014:** Event Driven

**Requirement:**
When `Role.String()` is called on a Role with role="readWrite" and db="admin", the Unit Test Suite SHALL return a string containing both the role and db values.

**Rationale:**
String representation is used in logging and error messages and MUST include all identifying fields.

**Verification:**
Create `Role{Role: "readWrite", Db: "admin"}`, call `String()`, assert output contains "readWrite" and "admin".

---

**TEST-015:** Event Driven

**Requirement:**
When `Privilege.String()` is called, the Unit Test Suite SHALL return a string containing the resource and actions values.

**Rationale:**
Privilege string representation MUST be human-readable for diagnostics.

**Verification:**
Create a Privilege with known Resource and Actions, call `String()`, assert output contains expected substrings.

---

**TEST-016:** Event Driven

**Requirement:**
When `Resource.String()` is called on a Resource with db and collection set, the Unit Test Suite SHALL return a string containing both the db and collection values.

**Rationale:**
Resource string representation MUST identify the target database and collection.

**Verification:**
Create `Resource{Db: "mydb", Collection: "mycol"}`, call `String()`, assert output contains "mydb" and "mycol".

---

### Replica Set Status Methods

**Test file:** `mongodb/replica_set_types_test.go`
**Source methods:** `ReplSetStatus.GetSelf()`, `ReplSetStatus.GetMembersByState()`, `ReplSetStatus.Primary()`

**TEST-017:** Event Driven

**Requirement:**
When `GetSelf` is called on a `ReplSetStatus` containing a member with `Self=true`, the Unit Test Suite SHALL return a pointer to that member.

**Rationale:**
Identifying the local member is required for replica set management operations.

**Verification:**
Construct ReplSetStatus with 3 members where member[1] has `Self: true`, call `GetSelf()`, assert returned member ID matches member[1].

---

**TEST-018:** Event Driven

**Requirement:**
When `GetSelf` is called on a `ReplSetStatus` where no member has `Self=true`, the Unit Test Suite SHALL return nil.

**Rationale:**
Callers MUST handle the case where the local node is not present in the status output.

**Verification:**
Construct ReplSetStatus with members all having `Self: false`, call `GetSelf()`, assert nil.

---

**TEST-019:** Event Driven

**Requirement:**
When `GetMembersByState` is called with a state and limit of 0, the Unit Test Suite SHALL return all members matching that state.

**Rationale:**
A limit of 0 indicates no limit and MUST return the complete set of matching members.

**Verification:**
Construct ReplSetStatus with 2 SECONDARY members and 1 PRIMARY, call `GetMembersByState(MemberStateSecondary, 0)`, assert 2 members returned.

---

**TEST-020:** Event Driven

**Requirement:**
When `GetMembersByState` is called with a positive limit, the Unit Test Suite SHALL return at most that many members matching the requested state.

**Rationale:**
The limit parameter MUST cap the result set for callers that only need one or a few members.

**Verification:**
Construct ReplSetStatus with 3 SECONDARY members, call `GetMembersByState(MemberStateSecondary, 1)`, assert exactly 1 member returned.

---

**TEST-021:** Event Driven

**Requirement:**
When `GetMembersByState` is called with a state that no member matches, the Unit Test Suite SHALL return an empty slice.

**Rationale:**
No match MUST produce an empty result, not nil or an error.

**Verification:**
Construct ReplSetStatus with only SECONDARY members, call `GetMembersByState(MemberStatePrimary, 0)`, assert empty slice.

---

**TEST-022:** Event Driven

**Requirement:**
When `Primary` is called on a `ReplSetStatus` containing a PRIMARY member, the Unit Test Suite SHALL return a pointer to that member.

**Rationale:**
`Primary()` is a convenience wrapper and MUST return the primary when one exists.

**Verification:**
Construct ReplSetStatus with 1 PRIMARY and 2 SECONDARY members, call `Primary()`, assert returned member state is `MemberStatePrimary`.

---

**TEST-023:** Event Driven

**Requirement:**
When `Primary` is called on a `ReplSetStatus` with no PRIMARY member, the Unit Test Suite SHALL return nil.

**Rationale:**
Callers MUST handle the no-primary case (e.g., during elections).

**Verification:**
Construct ReplSetStatus with only SECONDARY members, call `Primary()`, assert nil.

---

### Member State Constants

**Test file:** `mongodb/replica_set_types_test.go`
**Source:** `MemberStateStrings` map, `MemberState` and `MemberHealth` constants

**TEST-024:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that `MemberStateStrings` contains an entry for every defined `MemberState` constant.

**Rationale:**
Missing string mappings cause uninformative log output and can mask bugs.

**Verification:**
Iterate over all defined MemberState constants, assert each has a corresponding entry in `MemberStateStrings`.

---

**TEST-025:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that constant values `MinVotingMembers`, `MaxVotingMembers`, `MaxMembers`, `DefaultPriority`, `DefaultVotes` match expected replica set constraints (1, 7, 50, 2, 1 respectively).

**Rationale:**
These constants encode MongoDB replica set invariants and MUST not drift from the specification.

**Verification:**
Assert each constant equals its expected value.

---

### Validation Diagnostics Wrapper

**Test file:** `mongodb/helpers_test.go`
**Source function:** `validateDiagFunc`

**TEST-026:** Event Driven

**Requirement:**
When the wrapped validation function returns warnings and no errors, the Unit Test Suite SHALL verify that `validateDiagFunc` produces diagnostics with severity `DiagWarning` for each warning.

**Rationale:**
Warnings MUST be propagated to Terraform as warning-level diagnostics.

**Verification:**
Create a validation function returning `([]string{"warn1"}, nil)`, wrap with `validateDiagFunc`, invoke, assert 1 diagnostic with warning severity.

---

**TEST-027:** Event Driven

**Requirement:**
When the wrapped validation function returns errors and no warnings, the Unit Test Suite SHALL verify that `validateDiagFunc` produces diagnostics with severity `DiagError` for each error.

**Rationale:**
Errors MUST be propagated to Terraform as error-level diagnostics.

**Verification:**
Create a validation function returning `(nil, []error{fmt.Errorf("bad")})`, wrap with `validateDiagFunc`, invoke, assert 1 diagnostic with error severity.

---

**TEST-028:** Event Driven

**Requirement:**
When the wrapped validation function returns both warnings and errors, the Unit Test Suite SHALL produce diagnostics containing both warning and error entries.

**Rationale:**
Mixed results MUST not suppress either warnings or errors.

**Verification:**
Create a validation function returning 2 warnings and 1 error, wrap, invoke, assert 3 total diagnostics with correct severities.

---

**TEST-029:** Event Driven

**Requirement:**
When the wrapped validation function returns no warnings and no errors, the Unit Test Suite SHALL produce an empty diagnostics slice.

**Rationale:**
Valid input MUST not produce spurious diagnostics.

**Verification:**
Create a validation function returning `(nil, nil)`, wrap, invoke, assert empty diagnostics.

---

### Schema Validation

**Test file:** `mongodb/provider_test.go`
**Source function:** `Provider()`

**TEST-030:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that the provider schema returned by `Provider()` passes `InternalValidate()` without errors.

**Rationale:**
Schema misconfigurations (e.g., missing types, invalid defaults) MUST be caught before any acceptance testing.

**Verification:**
Call `Provider()`, invoke `InternalValidate()` on the returned `schema.Provider`, assert no error.

---

**TEST-031:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that the provider schema defines exactly 4 resources: `mongodb_db_user`, `mongodb_db_role`, `mongodb_shard_config`, and `mongodb_original_user`.

**Rationale:**
The resource map is the provider's public contract and MUST not silently gain or lose resources.

**Verification:**
Call `Provider()`, inspect `ResourcesMap`, assert exactly these 4 keys are present.

---

**TEST-032:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that each resource schema (`mongodb_db_user`, `mongodb_db_role`, `mongodb_shard_config`, `mongodb_original_user`) passes `InternalValidate()` without errors.

**Rationale:**
Individual resource schema errors MUST be caught independently.

**Verification:**
For each resource in `ResourcesMap`, call `InternalValidate()`, assert no error.

---

### BSON/JSON Serialization Round-Trips

**Test file:** `mongodb/config_test.go` and `mongodb/replica_set_types_test.go`
**Source types:** `Role`, `Resource`, `PrivilegeDto`, `DbUser`, `ConfigMember`, `RSConfig`, `Settings`, `ReplSetStatus`, `Member`

**TEST-033:** Event Driven

**Requirement:**
When a `Role` struct is marshaled to JSON and unmarshaled back, the Unit Test Suite SHALL verify that the resulting struct is equal to the original.

**Rationale:**
JSON tags on types MUST produce correct round-trip serialization for API communication.

**Verification:**
Create `Role{Role: "readWrite", Db: "admin"}`, marshal to JSON, unmarshal back, assert deep equality.

---

**TEST-034:** Event Driven

**Requirement:**
When a `Resource` struct is marshaled to JSON and unmarshaled back, the Unit Test Suite SHALL verify that the resulting struct is equal to the original, respecting `omitempty` tags for zero-value fields.

**Rationale:**
The `omitempty` tags on Resource fields MUST correctly omit zero-value fields during serialization.

**Verification:**
Create `Resource{Db: "test", Collection: ""}`, marshal to JSON, assert "collection" key is absent, unmarshal back, assert equality.

---

**TEST-035:** Event Driven

**Requirement:**
When a `ConfigMember` struct is marshaled to BSON and unmarshaled back, the Unit Test Suite SHALL verify that the resulting struct is equal to the original.

**Rationale:**
BSON tags are used for direct MongoDB wire protocol communication and MUST round-trip correctly.

**Verification:**
Create a `ConfigMember` with representative field values, marshal to BSON, unmarshal back, assert deep equality.

---

**TEST-036:** Event Driven

**Requirement:**
When an `RSConfig` struct containing `Settings` and `ConfigMembers` is marshaled to BSON and unmarshaled back, the Unit Test Suite SHALL verify the resulting struct is equal to the original.

**Rationale:**
The replica set configuration is the most complex BSON structure and MUST serialize correctly end-to-end.

**Verification:**
Create an RSConfig with populated Settings and multiple ConfigMembers, marshal to BSON, unmarshal back, assert deep equality.

---

**TEST-037:** Event Driven

**Requirement:**
When a `PrivilegeDto` struct with `Cluster: true` and empty Db/Collection is marshaled to JSON, the Unit Test Suite SHALL verify that the "db" and "collection" keys are absent from the output due to `omitempty`.

**Rationale:**
Cluster-scoped privileges MUST NOT include db/collection fields in API requests.

**Verification:**
Create `PrivilegeDto{Cluster: true, Actions: []string{"find"}}`, marshal to JSON, assert "db" and "collection" keys are absent.

---

### TLS Configuration Error Path

**Test file:** `mongodb/config_test.go`
**Source function:** `getTLSConfigWithAllServerCertificates`

**TEST-038:** Unwanted Behaviour

**Requirement:**
If `getTLSConfigWithAllServerCertificates` receives invalid PEM data, then the Unit Test Suite SHALL verify that it returns a non-nil error.

**Rationale:**
Invalid certificates MUST fail loudly rather than silently producing an empty cert pool.

**Verification:**
Call with `ca: []byte("not-a-pem-cert")` and `verify: false`, assert error is non-nil.

---

**TEST-039:** Event Driven

**Requirement:**
When `getTLSConfigWithAllServerCertificates` receives valid PEM data and `verify=true`, the Unit Test Suite SHALL verify that the returned `tls.Config` has `InsecureSkipVerify` set to `true`.

**Rationale:**
The verify parameter maps directly to `InsecureSkipVerify` and MUST be correctly propagated.

**Verification:**
Generate a self-signed PEM certificate in-test, call function with `verify: true`, assert `tlsConfig.InsecureSkipVerify == true`.

---

**TEST-040:** Event Driven

**Requirement:**
When `getTLSConfigWithAllServerCertificates` receives valid PEM data, the Unit Test Suite SHALL verify that the returned `tls.Config` has a non-nil `RootCAs` cert pool.

**Rationale:**
The certificate MUST be loaded into the RootCAs pool for TLS verification to work.

**Verification:**
Generate a self-signed PEM certificate in-test, call function, assert `tlsConfig.RootCAs` is non-nil.

---

### Client Construction (MongoClientNoAuth)

**Test file:** `mongodb/config_test.go`
**Source functions:** `mongoClientOptions`, `MongoClientNoAuth`, `MongoClient`

**TEST-041:** Event Driven

**Requirement:**
When `mongoClientOptions` is called with SSL, replica set, and retry writes configured, the Unit Test Suite SHALL verify that it returns non-nil options without error.

**Rationale:**
The shared options builder MUST produce valid options for all parameter combinations.

**Verification:**
Create a `ClientConfig` with Ssl=true, ReplicaSet="rs0", RetryWrites=true, Direct=false, call `mongoClientOptions`, assert non-nil and no error.

---

**TEST-042:** Event Driven

**Requirement:**
When `mongoClientOptions` is called with Direct=true and a non-empty ReplicaSet, the Unit Test Suite SHALL verify that it returns non-nil options without error.

**Rationale:**
Direct mode MUST suppress replica set discovery in the URI.

**Verification:**
Create a `ClientConfig` with Direct=true, ReplicaSet="rs0", call `mongoClientOptions`, assert non-nil and no error.

---

**TEST-043:** Event Driven

**Requirement:**
When `MongoClientNoAuth` is called, the Unit Test Suite SHALL verify that it returns a non-nil client without error.

**Rationale:**
The no-auth client path MUST produce a valid client for bootstrap connections.

**Verification:**
Create a minimal `ClientConfig`, call `MongoClientNoAuth`, assert non-nil client and nil error.

---

**TEST-044:** Event Driven

**Requirement:**
When `MongoClient` is called with username, password, and DB, the Unit Test Suite SHALL verify that it returns a non-nil client without error.

**Rationale:**
The authenticated client path MUST continue to work after the refactor into shared options.

**Verification:**
Create a `ClientConfig` with Username, Password, DB set, call `MongoClient`, assert non-nil client and nil error.

---

**TEST-045:** Event Driven

**Requirement:**
When `MongoClientNoAuth` is called with a valid PEM certificate, the Unit Test Suite SHALL verify that it returns a non-nil client without error.

**Rationale:**
TLS configuration MUST work with the no-auth client path for secure bootstrap connections.

**Verification:**
Generate a test PEM, create a `ClientConfig` with Certificate set, call `MongoClientNoAuth`, assert non-nil client and nil error.

---

### Original User Resource Schema

**Test file:** `mongodb/resource_original_user_test.go`
**Source function:** `resourceOriginalUser`

**TEST-046:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that the `mongodb_original_user` schema marks `host`, `port`, `username`, and `password` as Required.

**Rationale:**
The original user resource carries its own connection parameters and MUST require all four.

**Verification:**
Call `resourceOriginalUser()`, inspect schema for each field, assert `Required == true`.

---

**TEST-047:** Event Driven

**Requirement:**
When `resourceOriginalUser` schema is inspected, the Unit Test Suite SHALL verify that `auth_database` defaults to "admin", `direct` defaults to true, `ssl` defaults to false, and `insecure_skip_verify` defaults to false.

**Rationale:**
Defaults MUST match common MongoDB bootstrap scenarios.

**Verification:**
Call `resourceOriginalUser()`, inspect each field's Default value.

---

**TEST-048:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that the `password` field is marked Sensitive.

**Rationale:**
Passwords MUST be redacted from plan output and logs.

**Verification:**
Call `resourceOriginalUser()`, assert `Schema["password"].Sensitive == true`.

---

**TEST-049:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that the `certificate` field is marked Sensitive.

**Rationale:**
TLS certificates MUST be redacted from plan output and logs.

**Verification:**
Call `resourceOriginalUser()`, assert `Schema["certificate"].Sensitive == true`.

---

**TEST-050:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that the `role` field is TypeSet with nested `db` and `role` sub-fields.

**Rationale:**
Roles MUST follow the same schema pattern as `mongodb_db_user`.

**Verification:**
Call `resourceOriginalUser()`, inspect `Schema["role"]` type and nested Elem schema.

---

**TEST-051:** Ubiquitous

**Requirement:**
The Unit Test Suite SHALL verify that the `mongodb_original_user` schema passes `InternalValidate()`.

**Rationale:**
Schema misconfigurations MUST be caught before acceptance testing.

**Verification:**
Call `resourceOriginalUser()`, invoke `InternalValidate(nil, true)`, assert no error.

---

### Original User ID Parsing

**Test file:** `mongodb/resource_original_user_test.go`
**Source function:** `resourceOriginalUserParseId`

**TEST-052:** Event Driven

**Requirement:**
When `resourceOriginalUserParseId` receives a valid base64-encoded ID in "database.username" format, the Unit Test Suite SHALL return (username, database, nil).

**Rationale:**
ID parsing MUST correctly decompose the encoded ID for resource lookups.

**Verification:**
Encode "admin.myadmin" as base64, call `resourceOriginalUserParseId`, assert ("myadmin", "admin", nil).

---

**TEST-053:** Unwanted Behaviour

**Requirement:**
If `resourceOriginalUserParseId` receives invalid base64, then the Unit Test Suite SHALL return a non-nil error.

**Rationale:**
Corrupted state IDs MUST fail with a clear error.

**Verification:**
Call with "not-valid!@#", assert error is non-nil.

---

**TEST-054:** Unwanted Behaviour

**Requirement:**
If `resourceOriginalUserParseId` receives valid base64 without a "." separator, then the Unit Test Suite SHALL return a non-nil error.

**Rationale:**
Malformed IDs MUST be rejected.

**Verification:**
Encode "nodotshere" as base64, call, assert error is non-nil.

---

**TEST-055:** Unwanted Behaviour

**Requirement:**
If `resourceOriginalUserParseId` receives an ID with empty database or empty username parts, then the Unit Test Suite SHALL return a non-nil error.

**Rationale:**
Empty components indicate a corrupted ID and MUST be rejected.

**Verification:**
Test both ".username" and "database." encoded as base64, assert error for each.

---

**TEST-056:** Event Driven

**Requirement:**
When a `ClientConfig` is constructed for original user bootstrap with host, port, direct, and DB, the Unit Test Suite SHALL verify all fields are correctly set.

**Rationale:**
The resource's config builder MUST correctly populate the client config struct.

**Verification:**
Create a `ClientConfig` with known values, assert each field matches.

---

### Original User Resource EARS (Behavioral)

**Source file:** `mongodb/resource_original_user.go`

**ORIG-001:** Ubiquitous

**Requirement:**
The `mongodb_original_user` resource SHALL carry its own connection parameters (host, port, direct, ssl, certificate, insecure_skip_verify) independent of the provider config.

**Rationale:**
The provider requires auth credentials to connect. The original user resource MUST connect without auth, so it needs its own connection config.

---

**ORIG-002:** Event Driven

**Requirement:**
WHEN creating the original user, the resource SHALL connect to MongoDB WITHOUT authentication and create the specified user with the given roles.

**Rationale:**
A fresh MongoDB instance has no users and no auth. The bootstrap connection MUST be unauthenticated.

---

**ORIG-003:** Event Driven

**Requirement:**
WHEN `createUser` fails because the user already exists, the resource SHALL verify the user via an authenticated connection and adopt it into Terraform state.

**Rationale:**
Idempotent re-apply MUST not fail if the user was already created by a previous apply.

---

**ORIG-004:** Event Driven

**Requirement:**
WHEN reading the original user, the resource SHALL connect WITH authentication and verify the user exists.

**Rationale:**
After bootstrap, the server has auth enabled. Read MUST use credentials.

---

**ORIG-005:** Event Driven

**Requirement:**
WHEN updating the original user, the resource SHALL connect WITH authentication and recreate the user with new credentials or roles.

**Rationale:**
Updates to password or roles MUST be applied via an authenticated connection.

---

**ORIG-006:** Event Driven

**Requirement:**
WHEN deleting the original user, the resource SHALL connect WITH authentication and drop the user.

**Rationale:**
Cleanup MUST remove the user from MongoDB.

---

## Test File Summary

| Test File | Requirements | Source File |
|---|---|---|
| `mongodb/resource_db_user_test.go` | TEST-001 through TEST-004 | `mongodb/resource_db_user.go` |
| `mongodb/resource_db_role_test.go` | TEST-005, TEST-006 | `mongodb/resource_db_role.go` |
| `mongodb/resource_shard_config_test.go` | TEST-007, TEST-008 | `mongodb/resource_shard_config.go` |
| `mongodb/config_test.go` | TEST-009 through TEST-016, TEST-033, TEST-034, TEST-037 through TEST-045 | `mongodb/config.go` |
| `mongodb/replica_set_types_test.go` | TEST-017 through TEST-025, TEST-035, TEST-036 | `mongodb/replica_set_types.go` |
| `mongodb/helpers_test.go` | TEST-026 through TEST-029 | `mongodb/helpers.go` |
| `mongodb/provider_test.go` | TEST-030 through TEST-032 | `mongodb/provider.go` |
| `mongodb/resource_original_user_test.go` | TEST-046 through TEST-056 | `mongodb/resource_original_user.go` |
