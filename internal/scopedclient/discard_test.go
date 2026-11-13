package scopedclient_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
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
	discardable := lo.Filter(ent.ScopedEntities(), func(e *ent.ScopedEntity, _ int) bool { return e.Discardable })
	require.Subset(t, names(discardable), []string{"Integration", "SendingDomain", "Rule"})
	for _, e := range discardable {
		wrapper := e.Wrapper
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

// An entity neither archived nor discarded is deleted: its scoped wrapper has Delete and none of
// the operations of a row put aside (ADR 0049).
func TestDeletableEntityHasDeleteAndNoDiscardOrArchive(t *testing.T) {
	deletable := lo.Filter(ent.ScopedEntities(), func(e *ent.ScopedEntity, _ int) bool { return !e.PutAside() })
	require.Subset(t, names(deletable), []string{"Tag", "EmailTemplate", "WebhookEndpoint", "Membership"})
	for _, e := range deletable {
		wrapper := e.Wrapper
		for _, absent := range []string{"Discard", "QueryWithDiscarded", "Archive", "Unarchive", "QueryWithArchived", "Purge", "PurgeOneID"} {
			_, has := wrapper.MethodByName(absent)
			assert.Falsef(t, has, "%s has no %s", wrapper, absent)
		}
		for _, present := range []string{"Delete", "DeleteOneID"} {
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

	require.NoError(t, s.Integration().Discard(ctx, id, time.Now()))

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

	require.NoError(t, s.Integration().Discard(ctx, id, time.Now()))
	twice := s.Integration().Discard(ctx, id, time.Now())
	assert.True(t, ent.IsNotFound(twice), "already discarded")

	foreign := s.Integration().Discard(ctx, int64(fixtures.IntegrationGlobexID), time.Now())
	assert.True(t, ent.IsNotFound(foreign))
}
