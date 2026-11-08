package suspension_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An operator addresses a Workspace by its id (ADR 0043): the suspension's event tells the owner,
// once, and a second suspension keeps the first reason.
func TestSuspendAndUnsuspendAWorkspaceByItsID(t *testing.T) {
	env := testhelper.Setup(t)
	suspender := suspension.NewSuspender(env.Bus, time.Now)
	ctx := t.Context()

	changed, err := suspender.SuspendByPublicID(ctx, fixtures.KramericaPublicID, suspension.StaffMember("op-42"), "complaint rate")
	require.NoError(t, err)
	assert.True(t, changed)
	ws := env.DB.Workspace.GetX(ctx, fixtures.KramericaID)
	require.NotNil(t, ws.SuspendedAt)
	assert.Equal(t, workspace.SuspendedByKindStaff, *ws.SuspendedByKind)
	assert.Equal(t, "op-42", *ws.SuspendedByID)
	assert.Equal(t, "complaint rate", *ws.SuspensionReason)
	require.Len(t, env.QueuedEnvelopes(t, events.NameWorkspaceSuspended), 1, "the owner is told")

	changed, err = suspender.SuspendByPublicID(ctx, fixtures.KramericaPublicID, suspension.CLI, "again")
	require.NoError(t, err)
	assert.False(t, changed, "already suspended: nothing changes")
	assert.Len(t, env.QueuedEnvelopes(t, events.NameWorkspaceSuspended), 1, "and the owner is not told twice")
	assert.Equal(t, "complaint rate", *env.DB.Workspace.GetX(ctx, fixtures.KramericaID).SuspensionReason)

	changed, err = suspender.UnsuspendByPublicID(ctx, fixtures.KramericaPublicID, suspension.CLI)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Nil(t, env.DB.Workspace.GetX(ctx, fixtures.KramericaID).SuspendedAt)

	changed, err = suspender.UnsuspendByPublicID(ctx, fixtures.KramericaPublicID, suspension.CLI)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestSuspendAndUnsuspendByIDRejectBadInput(t *testing.T) {
	env := testhelper.Setup(t)
	suspender := suspension.NewSuspender(env.Bus, time.Now)
	ctx := t.Context()

	_, err := suspender.SuspendByPublicID(ctx, "NoSuchId", suspension.CLI, "why")
	require.ErrorContains(t, err, `workspace "NoSuchId"`)
	_, err = suspender.UnsuspendByPublicID(ctx, "NoSuchId", suspension.CLI)
	require.ErrorContains(t, err, `workspace "NoSuchId"`)

	changed, err := suspender.SuspendByPublicID(ctx, fixtures.KramericaPublicID, suspension.Actor{}, "reason")
	require.ErrorContains(t, err, "an actor is required")
	assert.False(t, changed)
	assert.Nil(t, env.DB.Workspace.GetX(ctx, fixtures.KramericaID).SuspendedAt)
}
