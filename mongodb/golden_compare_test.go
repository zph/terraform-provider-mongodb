//go:build integration

package mongodb

import (
	"os"
	"path/filepath"
	"testing"
)

// GOLDEN-006: WHEN UPDATE_GOLDEN env var is set, goldenCompare SHALL write the
// golden file instead of comparing.
// GOLDEN-007: WHEN the golden file does not exist, goldenCompare SHALL create
// it on first run.
// GOLDEN-008: WHEN the output differs from the golden file, goldenCompare SHALL
// fail the test with a diff message.

// goldenCompare compares data against a golden file in testdata/golden/.
// The filename is derived from t.Name() (e.g. TestGolden_DbUser_Basic → TestGolden_DbUser_Basic.golden).
// When UPDATE_GOLDEN is set, it writes the file instead of comparing.
func goldenCompare(t *testing.T, data string) {
	t.Helper()
	dir := filepath.Join("testdata", "golden")
	filename := t.Name() + ".golden"
	path := filepath.Join(dir, filename)

	if os.Getenv("UPDATE_GOLDEN") != "" {
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		err = os.WriteFile(path, []byte(data), 0o644)
		if err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		t.Logf("updated golden file %s", path)
		return
	}

	expected, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0o755)
		if err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		err = os.WriteFile(path, []byte(data), 0o644)
		if err != nil {
			t.Fatalf("write golden file: %v", err)
		}
		t.Logf("created golden file %s (first run)", path)
		return
	}
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}

	if string(expected) != data {
		t.Errorf("output differs from golden file %s; set UPDATE_GOLDEN=1 to update\n\n--- expected ---\n%s\n--- actual ---\n%s",
			filename, string(expected), data)
	}
}
