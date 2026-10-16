package mongodb

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

// errClassifierCase is one input to a CommandError classifier.
type errClassifierCase struct {
	name string
	err  error
	want bool
}

func checkErrClassifier(t *testing.T, classify func(error) bool, cases []errClassifierCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.err); got != tc.want {
				t.Errorf("want %v, got %v for %v", tc.want, got, tc.err)
			}
		})
	}
}

func TestIsNotYetInitialized(t *testing.T) {
	code94 := mongo.CommandError{Code: MongoErrNotYetInitialized, Message: "no replset config"}
	checkErrClassifier(t, IsNotYetInitialized, []errClassifierCase{
		{"INIT-T01 code 94", code94, true},
		{"INIT-T02 code 99", mongo.CommandError{Code: 99, Message: "other error"}, false},
		{"INIT-T03 plain error", fmt.Errorf("some plain error"), false},
		{"INIT-T04 wrapped code 94", errors.Wrap(code94, "replSetGetConfig"), true},
	})
}

func TestIsAlreadyInitialized(t *testing.T) {
	checkErrClassifier(t, IsAlreadyInitialized, []errClassifierCase{
		{"INIT-T05 code 23", mongo.CommandError{Code: MongoErrAlreadyInitialized, Message: "already initialized"}, true},
		{"INIT-T06 code 94", mongo.CommandError{Code: MongoErrNotYetInitialized, Message: "not yet initialized"}, false},
		{"INIT-T07 plain error", fmt.Errorf("some plain error"), false},
	})
}

func TestIsNotWriteReady(t *testing.T) {
	code17405 := mongo.CommandError{Code: MongoErrNotWriteReady, Message: "logOp() but can't accept write"}
	checkErrClassifier(t, IsNotWriteReady, []errClassifierCase{
		{"INIT-T12a code 17405", code17405, true},
		{"INIT-T12b unrelated code", mongo.CommandError{Code: 99, Message: "other error"}, false},
		{"INIT-T12c plain error", fmt.Errorf("some plain error"), false},
		{"INIT-T12d wrapped code 17405", errors.Wrap(code17405, "replSetReconfig"), true},
	})
}

func TestIsVersionConflict(t *testing.T) {
	code103 := mongo.CommandError{Code: MongoErrVersionConflict, Message: "version must be greater"}
	checkErrClassifier(t, IsVersionConflict, []errClassifierCase{
		{"INIT-T12e code 103", code103, true},
		{"INIT-T12f unrelated code", mongo.CommandError{Code: 99, Message: "other error"}, false},
		{"INIT-T12g wrapped code 103", errors.Wrap(code103, "replSetReconfig"), true},
	})
}

func TestIsAuthError(t *testing.T) {
	handshake := fmt.Errorf("connection() error occurred during connection handshake: auth error: sasl conversation error: unable to authenticate using mechanism \"SCRAM-SHA-1\": (AuthenticationFailed) Authentication failed.")
	// INIT-T12m: the wrapped handshake failure getShardClient returns.
	fromGetShardClient := fmt.Errorf("Error connecting to database: %w", handshake)
	checkErrClassifier(t, IsAuthError, []errClassifierCase{
		{"INIT-T12h AuthenticationFailed in error string", handshake, true},
		{"INIT-T12i Unauthorized code 13", mongo.CommandError{Code: MongoErrUnauthorized, Message: "not authorized on admin"}, true},
		{"INIT-T12j AuthenticationFailed code 18", mongo.CommandError{Code: MongoErrAuthenticationFailed, Message: "Authentication failed"}, true},
		{"INIT-T12k unrelated error", fmt.Errorf("network timeout"), false},
		{"INIT-T12l wrapped Unauthorized", errors.Wrap(mongo.CommandError{Code: MongoErrUnauthorized, Message: "not authorized"}, "replSetGetConfig"), true},
		{"INIT-T12m getShardClient format", fromGetShardClient, true},
	})
	// INIT-034: IsConnectionError must not claim a refused login.
	if IsConnectionError(fromGetShardClient) {
		t.Error("a refused login must not read as a connection error")
	}
}

// INIT-T13: INIT-032 — IsCurrentConfigNotCommitted matches code 308 only
func TestIsCurrentConfigNotCommitted(t *testing.T) {
	checkErrClassifier(t, IsCurrentConfigNotCommitted, []errClassifierCase{
		{"code 308", mongo.CommandError{Code: 308, Name: "CurrentConfigNotCommittedYet"}, true},
		{"wrapped code 308", fmt.Errorf("replSetReconfig: %w", mongo.CommandError{Code: 308}), true},
		{"code 103", mongo.CommandError{Code: 103}, false},
		{"plain error", errors.New("plain"), false},
	})
}

// INIT-T12, DISC-T21: the client-side-only shard_config attributes.
func TestShardConfigSchema_ClientSideFields(t *testing.T) {
	checkSchemaFields(t, resourceShardConfig(), []schemaField{
		{name: "init_timeout_secs", typ: schema.TypeInt, optional: true, def: DefaultInitTimeoutSecs},
		{name: "host_override", typ: schema.TypeString, optional: true},
	})
}
