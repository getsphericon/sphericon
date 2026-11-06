package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/workspacedeletion"
)

func deleteWorkspace(t *testing.T, c *siteapi.Client, id, name string) *siteapi.SiteDeletedWorkspaceResource {
	t.Helper()
	res, err := c.SiteWorkspacesDelete(context.Background(), &siteapi.SiteDeleteWorkspaceInput{Name: name},
		siteapi.SiteWorkspacesDeleteParams{WorkspaceId: siteapi.WorkspaceId(id)})
	require.NoError(t, err)
	return res
}

// tryDeleteWorkspace is deleteWorkspace for a test that expects the refusal.
func tryDeleteWorkspace(t *testing.T, c *siteapi.Client, id, name string) error {
	t.Helper()
	_, err := c.SiteWorkspacesDelete(context.Background(), &siteapi.SiteDeleteWorkspaceInput{Name: name},
		siteapi.SiteWorkspacesDeleteParams{WorkspaceId: siteapi.WorkspaceId(id)})
	return err
}

func TestOwnerDeletesWorkspaceByTypingItsName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	ctx := context.Background()

	deleted := deleteWorkspace(t, c, fixtures.InitechPublicID, "Initech")
	assert.Equal(t, siteapi.WorkspaceId(fixtures.InitechPublicID), deleted.ID)
	assert.WithinDuration(t, time.Now(), time.Time(deleted.DiscardedAt), time.Minute)
	assert.Equal(t, time.Time(deleted.DiscardedAt).Add(workspacedeletion.DeletionWindow), time.Time(deleted.EraseAt),
		"the data is erased 30 days after the deletion")

	list, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	assert.Empty(t, list, "a deleted Workspace leaves the list")

	// The paths of the deleted Workspace answers 404, for its Owner as well.
	_, err = c.SiteTagsList(ctx, siteapi.SiteTagsListParams{WorkspaceId: fixtures.InitechPublicID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestDeleteWithWrongNameIsUnprocessable(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)

	for _, typed := range []string{"initech", "Initech ", "", "Acme"} {
		testhelper.SiteRefusal(t, tryDeleteWorkspace(t, c, fixtures.InitechPublicID, typed), http.StatusUnprocessableEntity, "typed %q", typed)
	}
	list, err := c.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Len(t, list, 1, "nothing was deleted")
}

func TestOnlyAnOwnerDeletesAWorkspace(t *testing.T) {
	env := testhelper.Setup(t)

	for _, email := range []string{fixtures.InitechAdminAdaEmail, fixtures.InitechMemberMaxEmail} {
		testhelper.SiteRefusal(t, tryDeleteWorkspace(t, env.SiteActor(t, email), fixtures.InitechPublicID, "Initech"), http.StatusForbidden, email)
	}
	// Not a member at all: the Workspace is not found.
	testhelper.SiteRefusal(t, tryDeleteWorkspace(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.InitechPublicID, "Initech"), http.StatusNotFound)
}

func TestDeletedWorkspaceIsHiddenFromEveryMemberAndListedForOwners(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	owner := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	deleteWorkspace(t, owner, fixtures.InitechPublicID, "Initech")

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
	dora := env.SiteActor(t, fixtures.SoleOwnerDoraEmail)
	deleteWorkspace(t, dora, fixtures.AviatoPublicID, fixtures.AviatoName)

	list, err := dora.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.PiedPiperPublicID), list[0].ID)
}

func TestOwnerRestoresADeletedWorkspaceWithItsMembers(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	owner := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail)
	deleteWorkspace(t, owner, fixtures.InitechPublicID, "Initech")

	restored, err := owner.SiteWorkspacesRestore(ctx, siteapi.SiteWorkspacesRestoreParams{ID: fixtures.InitechPublicID})
	require.NoError(t, err)
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
	deleteWorkspace(t, env.SiteActor(t, fixtures.InitechOwnerOlgaEmail), fixtures.InitechPublicID, "Initech")
	params := siteapi.SiteWorkspacesRestoreParams{ID: fixtures.InitechPublicID}

	_, err := env.SiteActor(t, fixtures.InitechAdminAdaEmail).SiteWorkspacesRestore(ctx, params)
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "a Member cannot tell the deleted Workspace exists")

	_, err = env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesRestore(ctx, params)
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "a stranger cannot tell it exists")

	// Past the window the Workspace is no longer restorable.
	env.FeedDB.Workspace.UpdateOneID(fixtures.InitechID).
		SetDiscardedAt(time.Now().Add(-workspacedeletion.DeletionWindow - time.Hour)).ExecX(ctx)
	_, err = env.SiteActor(t, fixtures.InitechOwnerOlgaEmail).SiteWorkspacesRestore(ctx, params)
	testhelper.SiteRefusal(t, err, http.StatusConflict, "a Workspace past its window is a conflict")
	deleted, err := env.SiteActor(t, fixtures.InitechOwnerOlgaEmail).SiteWorkspacesListDeleted(ctx)
	require.NoError(t, err)
	assert.Empty(t, deleted)
}

func TestRestoreKeepsASuspensionInPlace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	sally := env.SiteActor(t, fixtures.OwnerSallyEmail) // Owner of Hooli, suspended by a Staff member
	deleteWorkspace(t, sally, fixtures.HooliPublicID, "Hooli")

	restored, err := sally.SiteWorkspacesRestore(ctx, siteapi.SiteWorkspacesRestoreParams{ID: fixtures.HooliPublicID})
	require.NoError(t, err)
	_, suspended := restored.SuspendedAt.Get()
	assert.True(t, suspended, "restoring never lifts a suspension")
}

func TestDeletingTheLastWorkspaceLeavesAnEmptyList(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	deleteWorkspace(t, c, fixtures.AcmePublicID, "Acme")

	list, err := c.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestDeleteAndRestoreAreAuditEntries(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	deleteWorkspace(t, c, fixtures.AcmePublicID, "Acme")
	_, err := c.SiteWorkspacesRestore(ctx, siteapi.SiteWorkspacesRestoreParams{ID: fixtures.AcmePublicID})
	require.NoError(t, err)

	for _, action := range []string{events.ActionWorkspaceDelete, events.ActionWorkspaceRestore} {
		got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, action)
		require.Len(t, got, 1, action)
		assert.Equal(t, siteapi.SiteAuditActorKindUser, got[0].Actor.Kind)
		assert.Equal(t, "Acme", got[0].Target.Name.Value)
	}
}
