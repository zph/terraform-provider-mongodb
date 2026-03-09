package cdktn

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goldenCompare compares data against a golden file in testdata/.
// The filename is derived from t.Name() (e.g. TestNewMongoShard_GoldenFile → TestNewMongoShard_GoldenFile.golden).
// When the UPDATE_GOLDEN env var is set, it writes the golden file instead.
func goldenCompare(t *testing.T, data []byte) {
	t.Helper()
	filename := t.Name() + ".golden"
	path := filepath.Join("testdata", filename)

	if os.Getenv("UPDATE_GOLDEN") != "" {
		err := os.MkdirAll("testdata", 0o755)
		require.NoError(t, err)
		err = os.WriteFile(path, data, 0o644)
		require.NoError(t, err)
		t.Logf("updated golden file %s", path)
		return
	}

	expected, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// First run — write the golden file
		err = os.MkdirAll("testdata", 0o755)
		require.NoError(t, err)
		err = os.WriteFile(path, data, 0o644)
		require.NoError(t, err)
		t.Logf("created golden file %s", path)
		return
	}
	require.NoError(t, err)
	assert.Equal(t, string(expected), string(data), "output differs from golden file %s; set UPDATE_GOLDEN=1 to update", filename)
}
