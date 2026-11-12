package site_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Every settings write of a Workspace names the revision it was made on (ADR 0119): a second change
// made on the revision the first one read is a 409 revision_changed, and the Workspace keeps the first.
func TestSiteWorkspaceSettingsOnAStaleRevisionAreRefused(t *testing.T) {
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	for name, change := range map[string]func(c *siteapi.Client, revision siteapi.Revision, first bool) error{
		"rename": func(c *siteapi.Client, revision siteapi.Revision, first bool) error {
			_, err := c.SiteWorkspacesUpdate(context.Background(), &siteapi.SiteUpdateWorkspaceInput{
				Revision: revision, Name: siteapi.Name(map[bool]string{true: "First", false: "Second"}[first]),
			}, siteapi.SiteWorkspacesUpdateParams{WorkspaceId: ws})
			return err
		},
		"time zone": func(c *siteapi.Client, revision siteapi.Revision, first bool) error {
			_, err := c.SiteWorkspacesSetTimezone(context.Background(), &siteapi.SiteWorkspaceTimezoneInput{
				Revision: revision, Timezone: siteapi.TimeZoneName(map[bool]string{true: "Europe/Berlin", false: "Asia/Tokyo"}[first]),
			}, siteapi.SiteWorkspacesSetTimezoneParams{WorkspaceId: ws})
			return err
		},
		"revenue settings": func(c *siteapi.Client, revision siteapi.Revision, first bool) error {
			_, err := c.SiteWorkspacesUpdateRevenueSettings(context.Background(), &siteapi.SiteUpdateRevenueSettingsInput{
				Revision: revision, AttributionWindowDays: siteapi.NewOptAttributionWindowDays(siteapi.AttributionWindowDays(map[bool]int32{true: 7, false: 9}[first])),
			}, siteapi.SiteWorkspacesUpdateRevenueSettingsParams{WorkspaceId: ws})
			return err
		},
		"domain join": func(c *siteapi.Client, revision siteapi.Revision, first bool) error {
			_, err := c.SiteDomainJoinSet(context.Background(), &siteapi.SiteSetDomainJoinInput{
				Revision: revision, Enabled: first,
			}, siteapi.SiteDomainJoinSetParams{WorkspaceId: ws})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.SiteActor(t, fixtures.OwnerJohnEmail)
			read := siteapi.Revision(env.WorkspaceRevision(context.Background(), t, fixtures.AcmePublicID))
			require.NoError(t, change(c, read, true))
			after := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)

			refused := testhelper.SiteRefusal(t, change(c, read, false), http.StatusConflict)

			assert.Equal(t, siteapi.ProblemCodeRevisionChanged, refused.Code.Value)
			now := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)
			assert.Equal(t, after.Revision, now.Revision, "the first change stays")
			assert.True(t, after.UpdatedAt.Equal(now.UpdatedAt), "nothing is written")
		})
	}
}

// A save that changes nothing writes nothing, so it leaves the revision as it was.
func TestSiteWorkspaceSaveThatChangesNothingKeepsTheRevision(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)

	saved, err := c.SiteWorkspacesUpdate(context.Background(), &siteapi.SiteUpdateWorkspaceInput{
		Revision: siteapi.Revision(before.Revision), Name: siteapi.Name(before.Name), PostalAddress: siteapi.NewOptString(before.PostalAddress),
	}, siteapi.SiteWorkspacesUpdateParams{WorkspaceId: fixtures.AcmePublicID})

	require.NoError(t, err)
	assert.EqualValues(t, before.Revision, saved.Revision)
}

// What the service keeps for itself on the Workspace row (schema.Bookkeeping) is written without
// moving the revision: a new Owner takes the service feed position, and the abuse detector stamps its
// warning. A settings form open meanwhile still saves.
func TestSiteWorkspaceBookkeepingLeavesTheRevision(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)

	_, err := owner.SiteMembershipsUpdate(context.Background(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleOwner},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeAdminMembershipID)})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	require.NoError(t, workspaces.MarkAbuseWarned(context.Background(), env.DB, fixtures.AcmeID, time.Now()))

	after := env.DB.Workspace.GetX(context.Background(), fixtures.AcmeID)
	require.Greater(t, after.FeedPosition, before.FeedPosition, "the new Owner took a feed position")
	require.NotNil(t, after.AbuseWarningNotifiedAt)
	assert.Equal(t, before.Revision, after.Revision)
	_, err = owner.SiteWorkspacesSetTimezone(context.Background(), &siteapi.SiteWorkspaceTimezoneInput{
		Revision: siteapi.Revision(before.Revision), Timezone: "Europe/Berlin",
	}, siteapi.SiteWorkspacesSetTimezoneParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
}
