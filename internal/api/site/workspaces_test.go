package site_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
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
		siteapi.SiteWorkspacesUpdateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	updated, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "Acme Inc", updated.Name)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.AcmePublicID), updated.ID, "the Workspace id is immutable")

	// The change is reflected in the list.
	list, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Acme Inc", list[0].Name)
}

// A single-Workspace response lists the active Staff holds like the list does.
func TestSiteWorkspacesUpdateResponseListsActiveStaffHolds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.ExtendHold(ctx, fixtures.AcmeID, fixtures.HoldAcmeLapsedID, time.Now().Add(time.Hour))
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteWorkspacesUpdate(ctx,
		&siteapi.SiteUpdateWorkspaceInput{Name: "Acme Inc"},
		siteapi.SiteWorkspacesUpdateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	updated, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)

	require.Len(t, updated.Holds, 1)
	assert.Equal(t, fixtures.HoldAcmeLapsedKey, updated.Holds[0].Key)
}

func TestSiteWorkspacesUpdateNotOwned(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteWorkspacesUpdate(context.Background(),
		&siteapi.SiteUpdateWorkspaceInput{Name: "Hijacked"},
		siteapi.SiteWorkspacesUpdateParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesUpdateNotFound{}, res)
}

func TestSiteWorkspacesUpdateRejectsBlankName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res, err := c.SiteWorkspacesUpdate(context.Background(),
		&siteapi.SiteUpdateWorkspaceInput{Name: "   "},
		siteapi.SiteWorkspacesUpdateParams{WorkspaceId: fixtures.AcmePublicID})
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

// The tracker snippet is built on the tracker's own host when the instance has one
// (SPHERICON_TRACKER_URL) and on the app origin otherwise.
func TestSiteWorkspacesListExposesTrackerURL(t *testing.T) {
	const trackerURL = "https://t.sphericon.example"
	ctx := context.Background()

	env := testhelper.Setup(t)
	list, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, env.AppURL, list[0].TrackerUrl, "without a tracker host the app origin serves it")

	split := testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.TrackerURL = trackerURL }))
	list, err = split.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesList(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, trackerURL, list[0].TrackerUrl)
}

// Every member sees the Staff holds on their Workspace (ADR 0009), told apart from a
// suspension: the resource lists the active holds with the setter's message as given, a
// lapsed hold is not listed, and an unaffected Workspace lists none.
func TestSiteWorkspacesListExposesActiveStaffHolds(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	const message = "Sending is paused for now."
	env.ExtendHold(ctx, fixtures.AcmeID, fixtures.HoldAcmeLapsedID, time.Now().Add(time.Hour))

	for _, email := range []string{fixtures.OwnerJohnEmail, fixtures.MemberMaryEmail} {
		acme := workspaceOf(t, env.SiteActor(t, email), fixtures.AcmePublicID)
		require.Len(t, acme.Holds, 1, "only the Acme hold is listed")
		assert.Equal(t, fixtures.HoldAcmeLapsedKey, acme.Holds[0].Key)
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
	assert.Empty(t, workspaceOf(t, c, fixtures.AcmePublicID).Holds, "only a lapsed hold exists")

	env.ExtendHold(context.Background(), fixtures.GlobexID, fixtures.HoldGlobexLapsedID, time.Now().Add(time.Hour))
	assert.Empty(t, workspaceOf(t, c, fixtures.AcmePublicID).Holds, "another Workspace's hold does not leak")
}

// Holds are honoured only with the `staff` license, on the send path and in what a
// Workspace is shown alike: an unlicensed instance lists none.
func TestSiteWorkspacesListShowsNoHoldsWithoutTheStaffLicense(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	env.ExtendHold(context.Background(), fixtures.AcmeID, fixtures.HoldAcmeLapsedID, time.Now().Add(time.Hour))
	assert.Empty(t, workspaceOf(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID).Holds)
}

func TestSiteWorkspacesListCarriesTheWorkspaceId(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	list, err := c.SiteWorkspacesList(context.Background())
	require.NoError(t, err)

	require.Len(t, list, 1)
	assert.Equal(t, siteapi.WorkspaceId(fixtures.AcmePublicID), list[0].ID)
}

func TestSiteWorkspacesCreateMakesTheCallerOwner(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	res, err := c.SiteWorkspacesCreate(ctx, &siteapi.SiteCreateWorkspaceInput{Name: "  Second Brand  "})
	require.NoError(t, err)
	created, ok := res.(*siteapi.SiteWorkspaceResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, "Second Brand", created.Name)
	assert.Equal(t, siteapi.SiteMembershipRoleOwner, created.Role)
	assert.NotEmpty(t, created.ID)
	assert.NotEqual(t, siteapi.WorkspaceId(fixtures.AcmePublicID), created.ID)

	list, err := c.SiteWorkspacesList(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 2)

	entries := entriesNamed(t, env, fixtures.OwnerJohnEmail, created.ID, events.ActionWorkspaceCreate)
	require.Len(t, entries, 1, "creation is an Audit entry of the new Workspace")
	name, _ := entries[0].Target.Name.Get()
	assert.Equal(t, "Second Brand", name)
}

func TestSiteWorkspacesCreateAllowsTwoWorkspacesWithOneName(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)

	ids := make([]siteapi.WorkspaceId, 0, 2)
	for range 2 {
		res, err := c.SiteWorkspacesCreate(t.Context(), &siteapi.SiteCreateWorkspaceInput{Name: "Acme"})
		require.NoError(t, err)
		created, ok := res.(*siteapi.SiteWorkspaceResource)
		require.Truef(t, ok, "got %T", res)
		ids = append(ids, created.ID)
	}
	assert.NotEqual(t, ids[0], ids[1])
}

func TestSiteWorkspacesCreateRefusesAnUnverifiedUser(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.FreshUnverifiedUserEmail)

	res, err := c.SiteWorkspacesCreate(t.Context(), &siteapi.SiteCreateWorkspaceInput{Name: "Sneaky"})
	require.NoError(t, err)
	refused, ok := res.(*siteapi.SiteWorkspacesCreateForbidden)
	require.Truef(t, ok, "got %T", res)
	code, _ := refused.Code.Get()
	assert.Equal(t, siteapi.ProblemCodeEmailVerificationRequired, code)
}

func TestSiteWorkspacesCreateRejectsBlankName(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWorkspacesCreate(t.Context(), &siteapi.SiteCreateWorkspaceInput{Name: " "})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesCreateUnprocessableEntity{}, res)
}

func TestSiteWorkspacesUpdateIsForOwnerAndAdminOnly(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	params := siteapi.SiteWorkspacesUpdateParams{WorkspaceId: fixtures.InitechPublicID}

	for _, email := range []string{fixtures.InitechOwnerOlgaEmail, fixtures.InitechAdminAdaEmail} {
		res, err := env.SiteActor(t, email).SiteWorkspacesUpdate(ctx, &siteapi.SiteUpdateWorkspaceInput{Name: "Initech " + email}, params)
		require.NoError(t, err)
		renamed, ok := res.(*siteapi.SiteWorkspaceResource)
		require.Truef(t, ok, "%s: got %T", email, res)
		assert.Equal(t, siteapi.WorkspaceId(fixtures.InitechPublicID), renamed.ID)
	}

	res, err := env.SiteActor(t, fixtures.InitechMemberMaxEmail).SiteWorkspacesUpdate(ctx, &siteapi.SiteUpdateWorkspaceInput{Name: "Mine now"}, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteWorkspacesUpdateForbidden{}, res)
	assert.Equal(t, "Initech "+fixtures.InitechAdminAdaEmail, workspaceOf(t, env.SiteActor(t, fixtures.InitechMemberMaxEmail), fixtures.InitechPublicID).Name)
}
