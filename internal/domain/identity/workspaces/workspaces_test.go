package workspaces_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestIDByPublicIDIn(t *testing.T) {
	env := testhelper.Setup(t)

	id, err := workspaces.IDByPublicIDIn(context.Background(), env.DB, fixtures.AcmePublicID)
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, id)
	_, err = workspaces.IDByPublicIDIn(context.Background(), env.DB, "NoSuchId")
	require.Error(t, err)
	_, err = workspaces.IDByPublicIDIn(context.Background(), env.DB, "acmews01")
	require.Error(t, err, "the id is case-sensitive")
}

func TestTheOldestLiveWorkspaceIsNeverADeletedOne(t *testing.T) {
	env := testhelper.Setup(t)
	a := workspaces.New(env.DB, env.Bus, time.Now)
	oldest, err := a.OldestLiveID(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, fixtures.AcmeID, oldest)

	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).ExecX(t.Context())

	next, err := a.OldestLiveID(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.GlobexID, next)
}

// A postal address is trimmed, so one of whitespace alone clears it, as an empty one does.
func TestUpdateTrimsThePostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	m := workspaces.New(env.DB, env.Bus, time.Now)
	s := env.DB.Scoped(fixtures.AcmeID)
	padded, blank := "  1 Main St  ", "   "

	set, err := m.Update(t.Context(), s, events.Actor{Kind: events.ActorUser}, env.DB.Workspace.GetX(t.Context(), fixtures.AcmeID).Revision, "Acme", &padded)
	require.NoError(t, err)
	cleared, err := m.Update(t.Context(), s, events.Actor{Kind: events.ActorUser}, set.Revision, "Acme", &blank)
	require.NoError(t, err)

	assert.Equal(t, "1 Main St", set.PostalAddress)
	assert.Empty(t, cleared.PostalAddress)
}
