package memberships_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/auditentry"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Workspace is created with its creator as its only Owner, the Send cap a Workspace starts with
// and the `workspace.create` entry in its own Audit log, the creator its actor.
func TestAWorkspaceIsCreatedWithItsOwnerItsStartingSendCapAndItsAuditEntry(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	startingCap := suspension.NewDefaultSendCap(env.DB, true)
	require.NoError(t, env.Bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		_, err := startingCap.SetIn(ctx, tx, new(500))
		return err
	}))
	creator := env.DB.User.GetX(ctx, fixtures.UmbrellaOwnerRitaID)
	ms := memberships.New(env.DB, env.Bus, startingCap, time.Now)

	var ws *ent.Workspace
	require.NoError(t, env.Bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		var err error
		ws, err = ms.CreateWorkspaceIn(ctx, tx, creator, "  Second Brand  ")
		return err
	}))
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	assert.Equal(t, "Second Brand", ws.Name)
	assert.Equal(t, 500, lo.FromPtr(ws.SendCap), "the Workspace starts with the default Send cap")
	members, err := env.DB.Membership.Query().Where(membership.WorkspaceID(ws.ID)).All(ctx)
	require.NoError(t, err)
	require.Len(t, members, 1)
	assert.EqualValues(t, fixtures.UmbrellaOwnerRitaID, members[0].UserID)
	assert.Equal(t, membership.RoleOwner, members[0].Role)
	assert.Equal(t, membership.SourceCreator, lo.FromPtr(members[0].Source))

	entry, err := env.DB.AuditEntry.Query().
		Where(auditentry.WorkspaceID(ws.ID), auditentry.Action(events.ActionWorkspaceCreate)).Only(ctx)
	require.NoError(t, err)
	assert.Equal(t, auditentry.ActorKindUser, entry.ActorKind)
	assert.Equal(t, creator.Name, lo.FromPtr(entry.ActorName))
	assert.Equal(t, "Second Brand", lo.FromPtr(entry.TargetName))
}

// An Unverified User creates no Workspace (ADR 0030), so sign-up verification is not bypassed by
// creating Workspaces first: nothing is written.
func TestAnUnverifiedUserCreatesNoWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	creator := env.DB.User.GetX(ctx, fixtures.FreshUnverifiedUserID)
	before := env.DB.Workspace.Query().CountX(ctx)

	ms := testhelper.Module[*memberships.Module](env)

	err := env.Bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		_, err := ms.CreateWorkspaceIn(ctx, tx, creator, "Fresh")
		return err
	})

	require.ErrorIs(t, err, memberships.ErrCreatorUnverified)
	assert.Equal(t, before, env.DB.Workspace.Query().CountX(ctx))
}
