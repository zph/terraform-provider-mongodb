package mongodb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
)

// GOLDEN-001: WHEN a MongoDB command is started, the CommandRecorder SHALL
// capture the command name, database, and BSON body.
// GOLDEN-002: WHEN a recorded command is one of the noise commands (hello,
// saslStart, saslContinue, ping, endSessions, isMaster, ismaster, buildInfo,
// getFreeMonitoringStatus, getLog), the CommandRecorder SHALL discard it.
// GOLDEN-003: WHEN rendering the BSON body to JSON, the CommandRecorder SHALL
// strip driver-injected fields ($db, $readPreference, lsid, $clusterTime).
// GOLDEN-004: WHEN the BSON body contains a "pwd" field, the CommandRecorder
// SHALL replace its value with "[REDACTED]".

// noiseCommands is the set of MongoDB commands filtered out by CommandRecorder.
var noiseCommands = map[string]bool{
	"hello":                   true,
	"saslStart":               true,
	"saslContinue":            true,
	"ping":                    true,
	"endSessions":             true,
	"isMaster":                true,
	"ismaster":                true,
	"buildInfo":               true,
	"getFreeMonitoringStatus": true,
	"getLog":                  true,
}

// driverMetaFields are fields injected by the MongoDB driver that add
// non-determinism to command bodies.
var driverMetaFields = map[string]bool{
	"$db":             true,
	"$readPreference": true,
	"lsid":            true,
	"$clusterTime":    true,
}

// recordedCommand holds one captured MongoDB command.
type recordedCommand struct {
	Name     string
	Database string
	Body     string // pretty-printed JSON
}

// CommandRecorder captures MongoDB commands via an event.CommandMonitor.
// It is safe for concurrent use.
type CommandRecorder struct {
	mu       sync.Mutex
	source   string // e.g. "TestGolden_DbUser_Basic (mongodb/golden_test.go)"
	commands []recordedCommand
}

// NewCommandRecorder creates a CommandRecorder with source provenance metadata.
// source identifies where commands originate from (test name + file).
func NewCommandRecorder(source string) *CommandRecorder {
	return &CommandRecorder{source: source}
}

// Monitor returns an event.CommandMonitor that records started commands,
// filtering out noise and stripping non-deterministic fields.
func (r *CommandRecorder) Monitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started: func(_ context.Context, e *event.CommandStartedEvent) {
			if noiseCommands[e.CommandName] {
				return
			}
			body, err := bsonToRedactedJSON(e.Command)
			if err != nil {
				body = fmt.Sprintf("<error: %v>", err)
			}
			r.mu.Lock()
			r.commands = append(r.commands, recordedCommand{
				Name:     e.CommandName,
				Database: e.DatabaseName,
				Body:     body,
			})
			r.mu.Unlock()
		},
	}
}

// String returns the deterministic multi-line representation of all recorded
// commands.
// GOLDEN-005: WHEN String() is called, the CommandRecorder SHALL produce
// output in the format "Source: <source>\nCommand: <name>\nDatabase: <db>\nBody:\n<json>"
// separated by blank lines. WHEN source is empty, the Source line SHALL be omitted.
func (r *CommandRecorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var sb strings.Builder
	for i, cmd := range r.commands {
		if i > 0 {
			sb.WriteString("\n")
		}
		if r.source != "" {
			fmt.Fprintf(&sb, "Source: %s\n", r.source)
		}
		fmt.Fprintf(&sb, "Command: %s\nDatabase: %s\nBody:\n%s\n", cmd.Name, cmd.Database, cmd.Body)
	}
	return sb.String()
}

// Reset clears all recorded commands.
func (r *CommandRecorder) Reset() {
	r.mu.Lock()
	r.commands = nil
	r.mu.Unlock()
}

// Commands returns a copy of the recorded commands.
func (r *CommandRecorder) Commands() []recordedCommand {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedCommand, len(r.commands))
	copy(out, r.commands)
	return out
}

// bsonToRedactedJSON converts raw BSON to pretty-printed JSON, stripping
// driver metadata fields and redacting password values.
func bsonToRedactedJSON(raw bson.Raw) (string, error) {
	if len(raw) == 0 {
		return "{}", nil
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		// The MongoDB driver redacts sensitive commands (createUser, etc.)
		// by sending an empty command body. Return a placeholder.
		return "<redacted by driver>", nil
	}

	cleaned := cleanDoc(doc)
	converted := bsonDToOrderedMap(cleaned)

	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(converted); err != nil {
		return "", fmt.Errorf("marshal JSON: %w", err)
	}
	// Encode appends a trailing newline; trim it.
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

// orderedMap preserves key insertion order for JSON marshaling.
type orderedMap struct {
	keys   []string
	values map[string]interface{}
}

func (o *orderedMap) MarshalJSON() ([]byte, error) {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		sb.Write(keyJSON)
		sb.WriteByte(':')
		valJSON, err := json.Marshal(o.values[k])
		if err != nil {
			return nil, err
		}
		sb.Write(valJSON)
	}
	sb.WriteByte('}')
	return []byte(sb.String()), nil
}

// bsonDToOrderedMap converts a bson.D to an orderedMap for proper JSON output.
func bsonDToOrderedMap(doc bson.D) *orderedMap {
	om := &orderedMap{
		keys:   make([]string, 0, len(doc)),
		values: make(map[string]interface{}, len(doc)),
	}
	for _, elem := range doc {
		om.keys = append(om.keys, elem.Key)
		om.values[elem.Key] = convertValue(elem.Value)
	}
	return om
}

// convertValue recursively converts bson types for JSON marshaling.
func convertValue(v interface{}) interface{} {
	switch val := v.(type) {
	case bson.D:
		return bsonDToOrderedMap(val)
	case bson.A:
		out := make([]interface{}, len(val))
		for i, item := range val {
			out[i] = convertValue(item)
		}
		return out
	default:
		return v
	}
}

// cleanDoc recursively processes a bson.D, removing driver meta fields and
// redacting pwd values.
func cleanDoc(doc bson.D) bson.D {
	var out bson.D
	for _, elem := range doc {
		if driverMetaFields[elem.Key] {
			continue
		}
		if elem.Key == "pwd" {
			out = append(out, bson.E{Key: elem.Key, Value: "[REDACTED]"})
			continue
		}
		out = append(out, bson.E{Key: elem.Key, Value: cleanValue(elem.Value)})
	}
	return out
}

// cleanValue recursively cleans nested documents and arrays.
func cleanValue(v interface{}) interface{} {
	switch val := v.(type) {
	case bson.D:
		return cleanDoc(val)
	case bson.A:
		out := make(bson.A, len(val))
		for i, item := range val {
			out[i] = cleanValue(item)
		}
		return out
	default:
		return v
	}
}

// --- Unit Tests ---

func TestCommandRecorder_FiltersNoise(t *testing.T) {
	rec := &CommandRecorder{}
	mon := rec.Monitor()

	for name := range noiseCommands {
		mon.Started(context.Background(), &event.CommandStartedEvent{
			Command:      mustBSON(bson.D{{Key: name, Value: 1}}),
			DatabaseName: "admin",
			CommandName:  name,
		})
	}

	if len(rec.Commands()) != 0 {
		t.Errorf("expected 0 recorded commands after noise, got %d", len(rec.Commands()))
	}
}

func TestCommandRecorder_RecordsNonNoise(t *testing.T) {
	rec := &CommandRecorder{}
	mon := rec.Monitor()

	mon.Started(context.Background(), &event.CommandStartedEvent{
		Command:      mustBSON(bson.D{{Key: "createUser", Value: "testuser"}, {Key: "pwd", Value: "secret"}, {Key: "roles", Value: bson.A{}}}),
		DatabaseName: "admin",
		CommandName:  "createUser",
	})

	cmds := rec.Commands()
	if len(cmds) != 1 {
		t.Fatalf("expected 1 command, got %d", len(cmds))
	}
	if cmds[0].Name != "createUser" {
		t.Errorf("expected command name 'createUser', got %q", cmds[0].Name)
	}
	if cmds[0].Database != "admin" {
		t.Errorf("expected database 'admin', got %q", cmds[0].Database)
	}
}

func TestCommandRecorder_RedactsPassword(t *testing.T) {
	rec := &CommandRecorder{}
	mon := rec.Monitor()

	mon.Started(context.Background(), &event.CommandStartedEvent{
		Command:      mustBSON(bson.D{{Key: "createUser", Value: "u1"}, {Key: "pwd", Value: "supersecret"}, {Key: "roles", Value: bson.A{}}}),
		DatabaseName: "testdb",
		CommandName:  "createUser",
	})

	output := rec.String()
	if strings.Contains(output, "supersecret") {
		t.Error("password was not redacted in output")
	}
	if !strings.Contains(output, "[REDACTED]") {
		t.Error("expected [REDACTED] placeholder in output")
	}
}

func TestCommandRecorder_StripsDriverMeta(t *testing.T) {
	rec := &CommandRecorder{}
	mon := rec.Monitor()

	mon.Started(context.Background(), &event.CommandStartedEvent{
		Command: mustBSON(bson.D{
			{Key: "find", Value: "users"},
			{Key: "$db", Value: "admin"},
			{Key: "$readPreference", Value: bson.D{{Key: "mode", Value: "primary"}}},
			{Key: "lsid", Value: bson.D{{Key: "id", Value: "abc123"}}},
			{Key: "$clusterTime", Value: bson.D{{Key: "clusterTime", Value: 1}}},
		}),
		DatabaseName: "admin",
		CommandName:  "find",
	})

	output := rec.String()
	for _, field := range []string{"$db", "$readPreference", "lsid", "$clusterTime"} {
		if strings.Contains(output, fmt.Sprintf("%q", field)) {
			t.Errorf("driver meta field %s was not stripped", field)
		}
	}
	if !strings.Contains(output, "find") {
		t.Error("expected 'find' command in output")
	}
}

func TestCommandRecorder_StringFormat(t *testing.T) {
	rec := &CommandRecorder{}
	mon := rec.Monitor()

	mon.Started(context.Background(), &event.CommandStartedEvent{
		Command:      mustBSON(bson.D{{Key: "dropUser", Value: "gone"}}),
		DatabaseName: "mydb",
		CommandName:  "dropUser",
	})

	output := rec.String()
	if !strings.HasPrefix(output, "Command: dropUser\n") {
		t.Errorf("unexpected output prefix: %q", output[:min(40, len(output))])
	}
	if !strings.Contains(output, "Database: mydb\n") {
		t.Error("expected 'Database: mydb' in output")
	}
	if !strings.Contains(output, "Body:\n") {
		t.Error("expected 'Body:' header in output")
	}
}

func TestCommandRecorder_StringFormatWithSource(t *testing.T) {
	rec := NewCommandRecorder("TestExample (mongodb/example_test.go)")
	mon := rec.Monitor()

	mon.Started(context.Background(), &event.CommandStartedEvent{
		Command:      mustBSON(bson.D{{Key: "dropUser", Value: "gone"}}),
		DatabaseName: "mydb",
		CommandName:  "dropUser",
	})

	output := rec.String()
	if !strings.HasPrefix(output, "Source: TestExample (mongodb/example_test.go)\n") {
		t.Errorf("expected Source line prefix, got: %q", output[:min(60, len(output))])
	}
	if !strings.Contains(output, "Command: dropUser\n") {
		t.Error("expected 'Command: dropUser' after Source line")
	}
}

func TestCommandRecorder_Reset(t *testing.T) {
	rec := &CommandRecorder{}
	mon := rec.Monitor()

	mon.Started(context.Background(), &event.CommandStartedEvent{
		Command:      mustBSON(bson.D{{Key: "find", Value: "col"}}),
		DatabaseName: "db",
		CommandName:  "find",
	})
	if len(rec.Commands()) != 1 {
		t.Fatal("expected 1 command before reset")
	}

	rec.Reset()
	if len(rec.Commands()) != 0 {
		t.Error("expected 0 commands after reset")
	}
}

func TestCommandRecorder_MultipleCommands(t *testing.T) {
	rec := &CommandRecorder{}
	mon := rec.Monitor()

	mon.Started(context.Background(), &event.CommandStartedEvent{
		Command:      mustBSON(bson.D{{Key: "createUser", Value: "u1"}, {Key: "pwd", Value: "p"}, {Key: "roles", Value: bson.A{}}}),
		DatabaseName: "admin",
		CommandName:  "createUser",
	})
	mon.Started(context.Background(), &event.CommandStartedEvent{
		Command:      mustBSON(bson.D{{Key: "usersInfo", Value: bson.D{{Key: "user", Value: "u1"}, {Key: "db", Value: "admin"}}}}),
		DatabaseName: "admin",
		CommandName:  "usersInfo",
	})

	output := rec.String()
	sections := strings.Split(output, "\nCommand: ")
	if len(sections) != 2 {
		t.Errorf("expected 2 command sections, got %d", len(sections))
	}
}

func TestBsonToRedactedJSON_NestedDocClean(t *testing.T) {
	doc := bson.D{
		{Key: "usersInfo", Value: bson.D{
			{Key: "user", Value: "test"},
			{Key: "db", Value: "admin"},
			{Key: "$db", Value: "should-strip"},
		}},
	}
	result, err := bsonToRedactedJSON(mustBSON(doc))
	if err != nil {
		t.Fatalf("bsonToRedactedJSON error: %v", err)
	}
	if strings.Contains(result, "should-strip") {
		t.Error("nested $db field was not stripped")
	}
}

// mustBSON marshals a bson.D to bson.Raw, panicking on error. Test-only helper.
func mustBSON(doc bson.D) bson.Raw {
	data, err := bson.Marshal(doc)
	if err != nil {
		panic(fmt.Sprintf("mustBSON: %v", err))
	}
	return bson.Raw(data)
}
