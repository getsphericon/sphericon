package schema_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mokevnin/sphericon/ent/migrate"
)

// Every entity table carries created_at and updated_at (ADR 0017); the pure
// edge table contact_tags is the only exception.
func TestEveryEntityTableHasTimestamps(t *testing.T) {
	checked := 0
	for _, table := range migrate.Tables {
		if table == migrate.ContactTagsTable {
			continue
		}
		checked++
		names := map[string]bool{}
		for _, c := range table.Columns {
			names[c.Name] = true
		}
		require.True(t, names["created_at"], "%s lacks created_at", table.Name)
		require.True(t, names["updated_at"], "%s lacks updated_at", table.Name)
	}
	require.Equal(t, 30, checked)
}
