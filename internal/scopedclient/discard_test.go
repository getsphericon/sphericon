package scopedclient_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An entity with the DiscardMixin is discarded for good: its scoped wrapper has Discard and no
// Delete, Undiscard, Restore or archive, and only Purge (Workspace erasure) removes it (ADR 0100).
func TestDiscardableEntityHasDiscardAndNoDeleteOrUndiscard(t *testing.T) {
	for _, wrapper := range []reflect.Type{
		reflect.TypeFor[*ent.IntegrationScoped](),
		reflect.TypeFor[*ent.SendingDomainScoped](),
		reflect.TypeFor[*ent.RuleScoped](),
	} {
		for _, absent := range []string{"Delete", "DeleteOneID", "Undiscard", "Restore", "QueryDiscarded", "Archive", "Unarchive", "QueryArchived", "QueryWithArchived"} {
			_, has := wrapper.MethodByName(absent)
			assert.Falsef(t, has, "%s has no %s", wrapper, absent)
		}
		for _, present := range []string{"Discard", "QueryWithDiscarded", "Purge", "PurgeOneID"} {
			_, has := wrapper.MethodByName(present)
			assert.Truef(t, has, "%s has %s", wrapper, present)
		}
	}
}

// The real archivable entities have no Delete and no Discard either.
func TestArchivableEntitiesHaveNoDeleteOrDiscard(t *testing.T) {
	for _, wrapper := range []reflect.Type{
		reflect.TypeFor[*ent.BroadcastScoped](),
		reflect.TypeFor[*ent.AutomationScoped](),
		reflect.TypeFor[*ent.SegmentScoped](),
	} {
		for _, absent := range []string{"Delete", "DeleteOneID", "Discard", "Undiscard", "Restore", "QueryWithDiscarded"} {
			_, has := wrapper.MethodByName(absent)
			assert.Falsef(t, has, "%s has no %s", wrapper, absent)
		}
		for _, present := range []string{"Archive", "Unarchive", "Purge"} {
			_, has := wrapper.MethodByName(present)
			assert.Truef(t, has, "%s has %s", wrapper, present)
		}
	}
}

// A discarded row leaves list queries but is still addressable by id, can be seen with
// QueryWithDiscarded.
func TestDiscardHidesFromListsAndKeepsByIDReads(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(acme)
	id := int64(fixtures.IntegrationAcmeGenericAID)

	require.NoError(t, s.Integration().Discard(ctx, id))

	listed, err := s.Integration().Query().Where(integration.ID(id)).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, listed, "a list query hides the discarded row")

	withDiscarded, err := s.Integration().QueryWithDiscarded().Where(integration.ID(id)).Exist(ctx)
	require.NoError(t, err)
	assert.True(t, withDiscarded)

	got, err := s.Integration().Get(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, got.DiscardedAt, "a by-id read sees the discarded row")
}

// Discard applies only to a kept row, and never across Workspaces.
func TestDiscardIsFencedByStateAndWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(acme)
	id := int64(fixtures.IntegrationAcmeGenericAID)

	require.NoError(t, s.Integration().Discard(ctx, id))
	twice := s.Integration().Discard(ctx, id)
	assert.True(t, ent.IsNotFound(twice), "already discarded")

	foreign := s.Integration().Discard(ctx, int64(fixtures.IntegrationGlobexID))
	assert.True(t, ent.IsNotFound(foreign))
}
