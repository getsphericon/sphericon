package scopedclient_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An entity with the DiscardMixin has no Delete on its scoped wrapper: a physical removal is
// a different, explicit method (Purge).
func TestDiscardableEntityHasNoDelete(t *testing.T) {
	wrapper := reflect.TypeFor[*ent.BroadcastScoped]()

	_, hasDelete := wrapper.MethodByName("Delete")
	_, hasDeleteOneID := wrapper.MethodByName("DeleteOneID")
	_, hasPurge := wrapper.MethodByName("Purge")

	assert.False(t, hasDelete)
	assert.False(t, hasDeleteOneID)
	assert.True(t, hasPurge)
}

// A discarded row leaves list queries but is still addressable by id, can be seen with
// QueryWithDiscarded, and comes back with Undiscard.
func TestDiscardHidesFromListsAndKeepsByIDReads(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(acme)
	id := int64(fixtures.BroadcastSentID)

	require.NoError(t, s.Broadcast().Discard(ctx, id))

	listed, err := s.Broadcast().Query().Where(broadcast.ID(id)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, listed, "a list query hides the discarded row")

	withDiscarded, err := s.Broadcast().QueryWithDiscarded().Where(broadcast.ID(id)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, withDiscarded)

	got, err := s.Broadcast().Get(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, got.DiscardedAt, "a by-id read sees the discarded row")

	require.NoError(t, s.Broadcast().Undiscard(ctx, id))

	listed, err = s.Broadcast().Query().Where(broadcast.ID(id)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, listed)
}

// Discard and Undiscard apply only to a row in the opposite state, and never across Workspaces.
func TestDiscardIsFencedByStateAndWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(acme)
	id := int64(fixtures.BroadcastSentID)

	notDiscarded := s.Broadcast().Undiscard(ctx, id)
	assert.True(t, ent.IsNotFound(notDiscarded), "nothing to restore on a kept row")

	require.NoError(t, s.Broadcast().Discard(ctx, id))
	twice := s.Broadcast().Discard(ctx, id)
	assert.True(t, ent.IsNotFound(twice), "already discarded")

	foreign := s.Broadcast().Discard(ctx, int64(fixtures.BroadcastGlobexID))
	assert.True(t, ent.IsNotFound(foreign))
}
