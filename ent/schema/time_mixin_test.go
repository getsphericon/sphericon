package schema_test

import (
	"testing"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/migrate"
)

// Every entity table carries created_at and updated_at (ADR 0017); the pure
// edge table contact_tags is the only exception.
func TestEveryEntityTableHasTimestamps(t *testing.T) {
	entities := lo.Without(migrate.Tables, migrate.ContactTagsTable)
	require.Len(t, entities, len(migrate.Tables)-1, "contact_tags is the one table skipped")

	for _, table := range entities {
		names := lo.Map(table.Columns, func(c *schema.Column, _ int) string { return c.Name })
		require.Contains(t, names, "created_at", "%s lacks created_at", table.Name)
		require.Contains(t, names, "updated_at", "%s lacks updated_at", table.Name)
	}
}
