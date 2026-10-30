package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

func deleteWorkspace(t *testing.T, c *siteapi.Client, id, name string) siteapi.SiteWorkspacesDeleteRes {
	t.Helper()
	res, err := c.SiteWorkspacesDelete(context.Background(), &siteapi.SiteDeleteWorkspaceInput{Name: name},
		siteapi.SiteWorkspacesDeleteParams{WorkspaceId: siteapi.WorkspaceId(id)})
	require.NoError(t, err)
	return res
}

func TestOwnerDeletesWorkspaceByTypingItsName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	ctx := context.Background()

	res := deleteWorkspace(t, c, fixtures.InitechPublicID, "Initech")
	deleted, ok := res.(*siteapi.SiteDeletedWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.InitechPublicID), deleted.ID)
	assert.WithinDuration(t, time.Now(), time.Time(deleted.DiscardedAt), time.Minute)
	assert.Equal(t, time.Time(deleted.DiscardedAt).Add(workspacedeletion.DeletionWindow), time.Time(deleted.EraseAt),
		"the data is erased 30 days after the deletion")

	list, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	assert.Empty(t, list, "a deleted Workspace leaves the list")

	// The paths of the deleted Workspace answers 404, for its Owner as well.
	tags, err := c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteTagsListNotFound{}, tags)
}

func TestDeleteWithWrongNameIsUnprocessable(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)

	for _, typed := range []string{"initech", "Initech ", "", "Acme"} {
		assert.IsType(t, &siteapi.SiteWorkspacesDeleteUnprocessableEntity{},
			deleteWorkspace(t, c, fixtures.InitechPublicID, typed), "typed %q", typed)
	}
	list, err := c.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1, "nothing was deleted")
}

func TestOnlyAnOwnerDeletesAWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	for _, email := range []string{fixtures.InitechAdminAdaEmail, fixtures.InitechMemberMaxEmail} {
		assert.IsType(t, &siteapi.SiteWorkspacesDeleteForbidden{},
			deleteWorkspace(t, env.SiteActor(t, email), fixtures.InitechPublicID, "Initech"), email)
	}
	// Not a member at all: the Workspace is not found.
	assert.IsType(t, &siteapi.SiteWorkspacesDeleteNotFound{},
		deleteWorkspace(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.InitechPublicID, "Initech"))
}

func TestDeletedWorkspaceIsHiddenFromEveryMemberAndListedForOwners(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	owner := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	require.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, deleteWorkspace(t, owner, fixtures.InitechPublicID, "Initech"))

	for _, email := range []string{fixtures.InitechAdminAdaEmail, fixtures.InitechMemberMaxEmail, fixtures.InitechCoOwnerOwenEmail} {
		c := env.SiteActor(t, email)
		list, err := c.SiteWorkspacesList(ctx)
		require.NoError(t, err)
		assert.Empty(t, list, email)
	}

	// Only an Owner sees it under Deleted workspaces, with the erase date: the co-owner too.
	for _, email := range []string{fixtures.InitechOwnerOlgaEmail, fixtures.InitechCoOwnerOwenEmail} {
		got, err := env.SiteActor(t, email).SiteWorkspacesListDeleted(ctx)
		require.NoError(t, err)
		require.Len(t, got, 1, email)
		assert.Equal(t, "Initech", got[0].Name)
		assert.Equal(t, time.Time(got[0].DiscardedAt).Add(workspacedeletion.DeletionWindow), time.Time(got[0].EraseAt))
		assert.False(t, got[0].OnlyOwner, "Initech has two Owners, either can restore it: "+email)
	}
	for _, email := range []string{fixtures.InitechAdminAdaEmail, fixtures.InitechMemberMaxEmail, fixtures.OwnerJohnEmail} {
		got, err := env.SiteActor(t, email).SiteWorkspacesListDeleted(ctx)
		require.NoError(t, err)
		assert.Empty(t, got, email)
	}
}

func TestADeletedWorkspaceOfItsOnlyOwnerSaysSo(t *testing.T) {
	env := testhelper.Setup(t)

	got, err := env.SiteActor(t, fixtures.DeletedWorkspaceOwnerDanEmail).SiteWorkspacesListDeleted(t.Context())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, fixtures.BluthName, got[0].Name)
	assert.True(t, got[0].OnlyOwner)
}

func TestDeletingOneWorkspaceLeavesTheOthers(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	created, err := john.SiteWorkspacesCreate(ctx, &siteapi.SiteCreateWorkspaceInput{Name: "Side project"})
	require.NoError(t, err)
	side, ok := created.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", created)
	require.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, deleteWorkspace(t, john, string(side.ID), "Side project"))

	list, err := john.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.AcmePublicID), list[0].ID)
}

func TestOwnerRestoresADeletedWorkspaceWithItsMembers(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	owner := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	require.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, deleteWorkspace(t, owner, fixtures.InitechPublicID, "Initech"))

	res, err := owner.SiteWorkspacesRestore(ctx, siteapi.SiteWorkspacesRestoreParams{WorkspaceId: fixtures.InitechPublicID})
	require.NoError(t, err)
	restored, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.SiteMembershipRoleOwner, restored.Role)

	deleted, err := owner.SiteWorkspacesListDeleted(ctx)
	require.NoError(t, err)
	assert.Empty(t, deleted)
	for _, email := range []string{fixtures.InitechOwnerOlgaEmail, fixtures.InitechAdminAdaEmail, fixtures.InitechMemberMaxEmail} {
		list, err := env.SiteActor(t, email).SiteWorkspacesList(ctx)
		require.NoError(t, err)
		require.Len(t, list, 1, email)
		assert.Equal(t, siteapi.WorkspaceId(fixtures.InitechPublicID), list[0].ID)
	}
}

func TestOnlyAnOwnerRestoresAndOnlyInsideTheWindow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	require.IsType(t, &siteapi.SiteDeletedWorkspaceResource{},
		deleteWorkspace(t, env.SiteActor(t, fixtures.InitechOwnerOlgaEmail), fixtures.InitechPublicID, "Initech"))
	params := siteapi.SiteWorkspacesRestoreParams{WorkspaceId: fixtures.InitechPublicID}

	res, err := env.SiteActor(t, fixtures.InitechAdminAdaEmail).SiteWorkspacesRestore(ctx, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesRestoreNotFound{}, res, "a Member cannot tell the deleted Workspace exists")

	res, err = env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesRestore(ctx, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesRestoreNotFound{}, res, "a stranger cannot tell it exists")

	// Past the window the Workspace is no longer restorable.
	env.DB.Workspace.UpdateOneID(fixtures.InitechID).
		SetDiscardedAt(time.Now().Add(-workspacedeletion.DeletionWindow - time.Hour)).ExecX(ctx)
	res, err = env.SiteActor(t, fixtures.InitechOwnerOlgaEmail).SiteWorkspacesRestore(ctx, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesRestoreConflict{}, res, "a Workspace past its window is a conflict")
	deleted, err := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail).SiteWorkspacesListDeleted(ctx)
	require.NoError(t, err)
	assert.Empty(t, deleted)
}

func TestRestoreKeepsASuspensionInPlace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	sally := env.SiteActor(t, fixtures.OwnerSallyEmail) // Owner of Hooli, suspended by a Staff member
	require.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, deleteWorkspace(t, sally, fixtures.HooliPublicID, "Hooli"))

	res, err := sally.SiteWorkspacesRestore(ctx, siteapi.SiteWorkspacesRestoreParams{WorkspaceId: fixtures.HooliPublicID})
	require.NoError(t, err)
	restored, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	_, suspended := restored.SuspendedAt.Get()
	assert.True(t, suspended, "restoring never lifts a suspension")
}

func TestDeletingTheLastWorkspaceLeavesAnEmptyList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	require.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, deleteWorkspace(t, c, fixtures.AcmePublicID, "Acme"))

	list, err := c.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestDeleteAndRestoreAreAuditEntries(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	require.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, deleteWorkspace(t, c, fixtures.AcmePublicID, "Acme"))
	_, err := c.SiteWorkspacesRestore(ctx, siteapi.SiteWorkspacesRestoreParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	for _, action := range []string{events.ActionWorkspaceDelete, events.ActionWorkspaceRestore} {
		got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, action)
		require.Len(t, got, 1, action)
		assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
		assert.Equal(t, "Acme", got[0].Target.Name.Value)
	}
}
