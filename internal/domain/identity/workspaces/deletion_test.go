package workspaces_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var deletedBy = events.Actor{Kind: events.ActorUser}

func TestDeletingTwiceKeepsTheFirstDeletionTime(t *testing.T) {
	env := testhelper.Setup(t)
	at := time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)
	m := workspaces.New(env.DB, env.Bus, func() time.Time { return at })
	s := env.DB.Scoped(fixtures.InitechID)
	name := env.DB.Workspace.GetX(t.Context(), fixtures.InitechID).Name

	first, err := m.Delete(t.Context(), s, deletedBy, name)
	require.NoError(t, err)
	at = at.Add(24 * time.Hour)
	second, err := m.Delete(t.Context(), s, deletedBy, name)
	require.NoError(t, err)

	require.NotNil(t, first.DiscardedAt)
	require.NotNil(t, second.DiscardedAt)
	assert.True(t, first.DiscardedAt.Equal(*second.DiscardedAt), "the window does not restart")
}

func TestDeleteRefusesAWrongName(t *testing.T) {
	env := testhelper.Setup(t)
	m := workspaces.New(env.DB, env.Bus, time.Now)

	_, err := m.Delete(t.Context(), env.DB.Scoped(fixtures.InitechID), deletedBy, "not the name")

	require.ErrorIs(t, err, workspaces.ErrNameMismatch)
	assert.Nil(t, env.DB.Workspace.GetX(t.Context(), fixtures.InitechID).DiscardedAt)
}
