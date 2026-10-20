package site_test

import (
	"context"
	"testing"
	"time"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/suspension"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteWorkspacesUpdateRenames(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteWorkspacesUpdate(ctx,
		&siteapi.SiteUpdateWorkspaceInput{Name: "Acme Inc"},
		siteapi.SiteWorkspacesUpdateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	updated, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "Acme Inc", updated.Name)
	assert.Equal(t, fixtures.AcmeSlug, updated.Slug, "slug is immutable")

	// The change is reflected in the list.
	list, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Acme Inc", list[0].Name)
}

func TestSiteWorkspacesUpdateNotOwned(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteWorkspacesUpdate(context.Background(),
		&siteapi.SiteUpdateWorkspaceInput{Name: "Hijacked"},
		siteapi.SiteWorkspacesUpdateParams{Slug: "does-not-exist"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesUpdateNotFound{}, res)
}

func TestSiteWorkspacesUpdateRejectsBlankName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteWorkspacesUpdate(context.Background(),
		&siteapi.SiteUpdateWorkspaceInput{Name: "   "},
		siteapi.SiteWorkspacesUpdateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesUpdateUnprocessableEntity{}, res)
}

// The owner sees a suspension (ADR 0007): the workspace resource carries when it was
// suspended and why, so the dashboard can say so while login and reads keep working.
func TestSiteWorkspacesListExposesSuspension(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	list, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	_, suspended := list[0].SuspendedAt.Get()
	assert.False(t, suspended, "a workspace that can send carries no suspension")

	_, err = suspension.SuspendWorkspace(ctx, env.Bus, 1, suspension.System, "complaint rate above 0.3%")
	require.NoError(t, err)

	list, err = c.SiteWorkspacesList(ctx)
	require.NoError(t, err, "reads keep working while suspended")
	at, suspended := list[0].SuspendedAt.Get()
	require.True(t, suspended)
	assert.False(t, time.Time(at).IsZero())
	reason, _ := list[0].SuspensionReason.Get()
	assert.Equal(t, "complaint rate above 0.3%", reason)
}

// Every member sees the Staff holds on their Workspace (ADR 0009), told apart from a
// suspension: the resource lists the active holds with the setter's message as given, a
// lapsed hold is not listed, and an unaffected Workspace lists none.
func TestSiteWorkspacesListExposesActiveStaffHolds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	const message = "Sending is paused for now."
	_, err := env.DB.Scoped(fixtures.AcmeID).Hold().Create().
		SetKey("k1").SetReasonCode("review").SetMessage(message).SetSetBy("service-a").
		SetExpiresAt(time.Now().Add(time.Hour)).Save(ctx)
	require.NoError(t, err)

	for _, email := range []string{fixtures.OwnerJohnEmail, fixtures.MemberMaryEmail} {
		acme := workspaceOf(t, env.SiteActor(t, email), fixtures.AcmeSlug)
		require.Len(t, acme.Holds, 1, "the lapsed fixture hold is not listed")
		assert.Equal(t, "k1", acme.Holds[0].Key)
		assert.Equal(t, "review", acme.Holds[0].ReasonCode)
		assert.Equal(t, message, acme.Holds[0].Message)
		assert.False(t, time.Time(acme.Holds[0].Since).IsZero())
		_, suspended := acme.SuspendedAt.Get()
		assert.False(t, suspended, "a hold is not a suspension")
	}
}

func TestSiteWorkspacesListShowsNoHoldsForAnUnaffectedWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	assert.Empty(t, workspaceOf(t, c, fixtures.AcmeSlug).Holds, "only a lapsed hold exists")

	_, err := env.DB.Scoped(fixtures.GlobexID).Hold().Create().
		SetKey("k1").SetReasonCode("review").SetMessage("held").SetSetBy("service-a").
		SetExpiresAt(time.Now().Add(time.Hour)).Save(context.Background())
	require.NoError(t, err)
	assert.Empty(t, workspaceOf(t, c, fixtures.AcmeSlug).Holds, "another Workspace's hold does not leak")
}

// Holds are honoured only with the `staff` license, on the send path and in what a
// Workspace is shown alike: an unlicensed instance lists none.
func TestSiteWorkspacesListShowsNoHoldsWithoutTheStaffLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	_, err := env.DB.Scoped(fixtures.AcmeID).Hold().Create().
		SetKey("k1").SetReasonCode("review").SetMessage("held").SetSetBy("service-a").
		SetExpiresAt(time.Now().Add(time.Hour)).Save(context.Background())
	require.NoError(t, err)
	assert.Empty(t, workspaceOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmeSlug).Holds)
}

func TestSiteWorkspacesListCarriesTheWorkspaceId(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	list, err := c.SiteWorkspacesList(context.Background())
	require.NoError(t, err)

	require.Len(t, list, 1)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.AcmePublicID), list[0].PublicId)
}
