package versioneddefinition_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestAnArchivableVersionedStoreArchivesInactiveAndNeverDiscards(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	id := int64(fixtures.AutomationWinBackID)
	var store ent.VersionedStore[ent.Automation, ent.AutomationVersion] = acme(env).Automation().Pointers()
	before, err := store.Read(ctx, id)
	require.NoError(t, err)
	after := before.VersionPosition
	after.Active = false
	after.Archived = true

	swapped, err := store.CompareAndSwap(ctx, before, after, time.Now())

	require.NoError(t, err)
	assert.True(t, swapped)
	archived, err := store.Read(ctx, id)
	require.NoError(t, err)
	assert.True(t, archived.Archived)
	assert.False(t, archived.Active)
	assert.False(t, archived.Discarded)
	restored := archived.VersionPosition
	restored.Archived = false
	swapped, err = store.CompareAndSwap(ctx, archived, restored, time.Now())
	require.NoError(t, err)
	assert.True(t, swapped)
	kept, err := store.Read(ctx, id)
	require.NoError(t, err)
	assert.False(t, kept.Archived)
	restored.Discarded = true
	_, err = store.CompareAndSwap(ctx, kept, restored, time.Now())
	require.ErrorIs(t, err, ent.ErrNotDiscardable)
}

// A stale publication cannot revive an object another writer archived, even though the old
// and new positions have the same live pointer. The complete position is the conditional write.
func TestAVersionedStoreRefusesAStalePosition(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	id := int64(fixtures.AutomationWinBackID)
	store := acme(env).Automation().Pointers()
	before, err := store.Read(ctx, id)
	require.NoError(t, err)
	archived := before.VersionPosition
	archived.Active = false
	archived.Archived = true
	swapped, err := store.CompareAndSwap(ctx, before, archived, time.Now())
	require.NoError(t, err)
	require.True(t, swapped)

	swapped, err = store.CompareAndSwap(ctx, before, before.VersionPosition, time.Now())

	require.NoError(t, err)
	assert.False(t, swapped)
	stored, err := store.Read(ctx, id)
	require.NoError(t, err)
	assert.True(t, stored.Archived)
	assert.False(t, stored.Active)
}

// Even a writer leaving status and pointers unchanged invalidates an older snapshot. This
// fences a rewrite of the existing draft against another move that read that draft earlier.
func TestAVersionedStoreRefusesAStaleRevisionWithTheSamePosition(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	id := int64(fixtures.AutomationWinBackID)
	store := acme(env).Automation().Pointers()
	before, err := store.Read(ctx, id)
	require.NoError(t, err)
	swapped, err := store.CompareAndSwap(ctx, before, before.VersionPosition, time.Now())
	require.NoError(t, err)
	require.True(t, swapped)

	swapped, err = store.CompareAndSwap(ctx, before, before.VersionPosition, time.Now())

	require.NoError(t, err)
	assert.False(t, swapped)
}
