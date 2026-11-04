package drafts_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The storage half of an archivable Versioned definition (the Automation) archives through
// archived_at, reports it as Archived (never Discarded), and refuses to discard.
func TestAnArchivableVersionedStoreArchivesAndNeverDiscards(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	id := int64(fixtures.AutomationWinBackID)
	var store ent.VersionedStore[ent.Automation, ent.AutomationVersion] = acme(env).Automation().Pointers()

	require.NoError(t, store.Archive(ctx, id))
	archived, err := store.Lock(ctx, id)
	require.NoError(t, err)
	require.NoError(t, store.Unarchive(ctx, id))
	kept, err := store.Lock(ctx, id)
	require.NoError(t, err)

	assert.True(t, archived.Archived)
	assert.False(t, archived.Discarded)
	assert.False(t, kept.Archived)
	require.ErrorIs(t, store.Discard(ctx, id), ent.ErrNotDiscardable)
}
