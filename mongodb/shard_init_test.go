package mongodb

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

// --- IsNotYetInitialized tests ---

// INIT-T01: IsNotYetInitialized true for mongo.CommandError{Code: 94}
func TestIsNotYetInitialized_Code94(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrNotYetInitialized, Message: "no replset config"}
	if !IsNotYetInitialized(err) {
		t.Error("expected true for CommandError code 94")
	}
}

// INIT-T02: IsNotYetInitialized false for mongo.CommandError{Code: 99}
func TestIsNotYetInitialized_Code99(t *testing.T) {
	err := mongo.CommandError{Code: 99, Message: "other error"}
	if IsNotYetInitialized(err) {
		t.Error("expected false for CommandError code 99")
	}
}

// INIT-T03: IsNotYetInitialized false for plain fmt.Errorf
func TestIsNotYetInitialized_PlainError(t *testing.T) {
	err := fmt.Errorf("some plain error")
	if IsNotYetInitialized(err) {
		t.Error("expected false for plain error")
	}
}

// INIT-T04: IsNotYetInitialized true for pkgerrors.Wrap(CommandError{94})
func TestIsNotYetInitialized_WrappedCode94(t *testing.T) {
	inner := mongo.CommandError{Code: MongoErrNotYetInitialized, Message: "no replset config"}
	wrapped := errors.Wrap(inner, "replSetGetConfig")
	if !IsNotYetInitialized(wrapped) {
		t.Error("expected true for wrapped CommandError code 94")
	}
}

// --- IsAlreadyInitialized tests ---

// INIT-T05: IsAlreadyInitialized true for mongo.CommandError{Code: 23}
func TestIsAlreadyInitialized_Code23(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrAlreadyInitialized, Message: "already initialized"}
	if !IsAlreadyInitialized(err) {
		t.Error("expected true for CommandError code 23")
	}
}

// INIT-T06: IsAlreadyInitialized false for code 94
func TestIsAlreadyInitialized_Code94(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrNotYetInitialized, Message: "not yet initialized"}
	if IsAlreadyInitialized(err) {
		t.Error("expected false for CommandError code 94")
	}
}

// INIT-T07: IsAlreadyInitialized false for plain error
func TestIsAlreadyInitialized_PlainError(t *testing.T) {
	err := fmt.Errorf("some plain error")
	if IsAlreadyInitialized(err) {
		t.Error("expected false for plain error")
	}
}

// --- IsNotWriteReady tests ---

// INIT-T12a: IsNotWriteReady true for mongo.CommandError{Code: 17405}
func TestIsNotWriteReady_Code17405(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrNotWriteReady, Message: "logOp() but can't accept write"}
	if !IsNotWriteReady(err) {
		t.Error("expected true for CommandError code 17405")
	}
}

// INIT-T12b: IsNotWriteReady false for unrelated code
func TestIsNotWriteReady_UnrelatedCode(t *testing.T) {
	err := mongo.CommandError{Code: 99, Message: "other error"}
	if IsNotWriteReady(err) {
		t.Error("expected false for CommandError code 99")
	}
}

// INIT-T12c: IsNotWriteReady false for plain error
func TestIsNotWriteReady_PlainError(t *testing.T) {
	err := fmt.Errorf("some plain error")
	if IsNotWriteReady(err) {
		t.Error("expected false for plain error")
	}
}

// INIT-T12d: IsNotWriteReady true for wrapped CommandError{17405}
func TestIsNotWriteReady_WrappedCode17405(t *testing.T) {
	inner := mongo.CommandError{Code: MongoErrNotWriteReady, Message: "logOp() but can't accept write"}
	wrapped := errors.Wrap(inner, "replSetReconfig")
	if !IsNotWriteReady(wrapped) {
		t.Error("expected true for wrapped CommandError code 17405")
	}
}

// --- IsVersionConflict tests ---

// INIT-T12e: IsVersionConflict true for mongo.CommandError{Code: 103}
func TestIsVersionConflict_Code103(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrVersionConflict, Message: "version must be greater"}
	if !IsVersionConflict(err) {
		t.Error("expected true for CommandError code 103")
	}
}

// INIT-T12f: IsVersionConflict false for unrelated code
func TestIsVersionConflict_UnrelatedCode(t *testing.T) {
	err := mongo.CommandError{Code: 99, Message: "other error"}
	if IsVersionConflict(err) {
		t.Error("expected false for CommandError code 99")
	}
}

// INIT-T12g: IsVersionConflict true for wrapped CommandError{103}
func TestIsVersionConflict_Wrapped(t *testing.T) {
	inner := mongo.CommandError{Code: MongoErrVersionConflict, Message: "version must be greater"}
	wrapped := errors.Wrap(inner, "replSetReconfig")
	if !IsVersionConflict(wrapped) {
		t.Error("expected true for wrapped CommandError code 103")
	}
}

// --- IsAuthError tests ---

// INIT-T12h: IsAuthError true for AuthenticationFailed in error string
func TestIsAuthError_AuthenticationFailed(t *testing.T) {
	err := fmt.Errorf("connection() error occurred during connection handshake: auth error: sasl conversation error: unable to authenticate using mechanism \"SCRAM-SHA-1\": (AuthenticationFailed) Authentication failed.")
	if !IsAuthError(err) {
		t.Error("expected true for AuthenticationFailed error")
	}
}

// INIT-T12i: IsAuthError true for Unauthorized command error (code 13)
func TestIsAuthError_Unauthorized(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrUnauthorized, Message: "not authorized on admin"}
	if !IsAuthError(err) {
		t.Error("expected true for CommandError code 13")
	}
}

// INIT-T12j: IsAuthError true for AuthenticationFailed command error (code 18)
func TestIsAuthError_Code18(t *testing.T) {
	err := mongo.CommandError{Code: MongoErrAuthenticationFailed, Message: "Authentication failed"}
	if !IsAuthError(err) {
		t.Error("expected true for CommandError code 18")
	}
}

// INIT-T12k: IsAuthError false for unrelated error
func TestIsAuthError_UnrelatedError(t *testing.T) {
	err := fmt.Errorf("network timeout")
	if IsAuthError(err) {
		t.Error("expected false for unrelated error")
	}
}

// INIT-T12l: IsAuthError true for wrapped Unauthorized
func TestIsAuthError_WrappedUnauthorized(t *testing.T) {
	inner := mongo.CommandError{Code: MongoErrUnauthorized, Message: "not authorized"}
	wrapped := errors.Wrap(inner, "replSetGetConfig")
	if !IsAuthError(wrapped) {
		t.Error("expected true for wrapped Unauthorized")
	}
}

// --- diagContainsAuthError tests ---

// INIT-T12m: diagContainsAuthError matches diag.Errorf from getShardClient
func TestDiagContainsAuthError_GetShardClientFormat(t *testing.T) {
	d := diag.Errorf("Error connecting to database: connection() error occurred during connection handshake: auth error: sasl conversation error: unable to authenticate using mechanism \"SCRAM-SHA-1\": (AuthenticationFailed) Authentication failed.")
	if !diagContainsAuthError(d) {
		t.Errorf("expected diagContainsAuthError to match getShardClient auth error, got false. Summary=%q Detail=%q", d[0].Summary, d[0].Detail)
	}
}

// INIT-T12: Schema: init_timeout_secs exists, Optional, Default 60
func TestShardConfigSchema_InitTimeoutSecs(t *testing.T) {
	res := resourceShardConfig()
	field, ok := res.Schema["init_timeout_secs"]
	if !ok {
		t.Fatal("schema missing 'init_timeout_secs' field")
	}
	if field.Required {
		t.Error("init_timeout_secs should be Optional, not Required")
	}
	if field.Default != DefaultInitTimeoutSecs {
		t.Errorf("init_timeout_secs default: want %d, got %v", DefaultInitTimeoutSecs, field.Default)
	}
}
