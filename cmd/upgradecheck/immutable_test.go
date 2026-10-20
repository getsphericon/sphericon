package main

import (
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func migrationName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		stem := rapid.StringMatching(`[0-9]{14}_[a-z_]{1,12}`).Draw(t, "stem")
		return "migrations/" + stem + rapid.SampledFrom([]string{".sql", ".go"}).Draw(t, "ext")
	})
}

func TestImmutabilityReportsExactlyModifiedAndDeleted(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		modified := rapid.SliceOf(migrationName()).Draw(t, "modified")
		deleted := rapid.SliceOf(migrationName()).Draw(t, "deleted")
		// Added files are never passed in; draw names distinct from the changed ones.
		changed := slices.Concat(modified, deleted)
		added := lo.Without(rapid.SliceOf(migrationName()).Draw(t, "added"), changed...)

		got := Immutability(modified, deleted, false)

		assert.Equal(t, slices.Compact(slices.Sorted(slices.Values(changed))), got)
		assert.Empty(t, lo.Intersect(got, added))
		assert.Empty(t, Immutability(modified, deleted, true))
	})
}

func TestImmutabilityIgnoresBookkeepingFiles(t *testing.T) {
	got := Immutability([]string{"migrations/README.md", "docs/x.sql", "migrations/sub/a.sql"}, []string{"migrations/README.md"}, false)
	assert.Empty(t, got)
}

func TestCheckImmutability(t *testing.T) {
	require.NoError(t, CheckImmutability(nil, nil, false))
	err := CheckImmutability([]string{"migrations/1_a.sql"}, []string{"migrations/2_b.sql"}, false)
	require.ErrorContains(t, err, "migrations/1_a.sql")
	require.ErrorContains(t, err, "migrations/2_b.sql")
	require.NoError(t, CheckImmutability([]string{"migrations/1_a.sql"}, nil, true))
}

func TestIsHistoryCollapse(t *testing.T) {
	assert.True(t, IsHistoryCollapse("text\n\nHistory-Collapse: squash to a baseline"))
	assert.False(t, IsHistoryCollapse("History-Collapse:"))
	assert.False(t, IsHistoryCollapse("see History-Collapse: in prose"))
}
