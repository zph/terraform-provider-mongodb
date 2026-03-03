package mongodb

import (
	"fmt"
	"strings"
)

// parseResourceId splits a plain text resource ID of the form "database.name"
// into its components. It returns (name, database, error).
// IDFORMAT-002, IDFORMAT-003
func parseResourceId(id string) (string, string, error) {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("unexpected format of ID (%s), expected database.name", id)
	}
	return parts[1], parts[0], nil
}

// formatResourceId returns a plain text resource ID "database.name".
// IDFORMAT-004
func formatResourceId(database, name string) string {
	return database + "." + name
}
