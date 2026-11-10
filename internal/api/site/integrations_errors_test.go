package site_test

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integration"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteIntegrationsErrorBranches(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	acme, foreign := siteapi.WorkspaceId(fixtures.AcmePublicID), siteapi.WorkspaceId(fixtures.GlobexPublicID)

	id := siteapi.EntityId(fixtures.IntegrationAcmeGenericAID)
	total, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Count(ctx)
	require.NoError(t, err)

	t.Run("list is scoped and requires a workspace", func(t *testing.T) {
		list, err := c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: acme})
		require.NoError(t, err)
		require.Len(t, list, total, "the Workspace's integrations")
		assert.True(t, slices.IsSortedFunc(list, func(a, b siteapi.SiteIntegrationResource) int {
			return cmp.Compare(a.ID, b.ID)
		}), "ordered by id")

		_, err = c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: foreign})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)

		_, err = env.SiteAnonymous(t).SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: acme})
		require.Error(t, err)
	})

	t.Run("create validates", func(t *testing.T) {
		_, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{Name: "x", Config: smtpInput("h", "p")},
			siteapi.SiteIntegrationsCreateParams{WorkspaceId: foreign})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{Name: "   ", Config: smtpInput("h", "p")},
			siteapi.SiteIntegrationsCreateParams{WorkspaceId: acme})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		_, err = c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{Name: "bad", Config: smtpInput("", "p")},
			siteapi.SiteIntegrationsCreateParams{WorkspaceId: acme})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity, "an SMTP config without a host is rejected")
		n, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, total, n, "rejected creates persisted nothing")
	})

	t.Run("get", func(t *testing.T) {
		_, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: foreign, ID: id})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: acme, ID: idStr(99999)})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
	})

	t.Run("update", func(t *testing.T) {
		in := &siteapi.UpdateIntegrationInput{Name: siteapi.NewOptName("Renamed")}
		_, err := c.SiteIntegrationsUpdate(ctx, in, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: foreign, ID: id})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteIntegrationsUpdate(ctx, in, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: acme, ID: idStr(99999)})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Name: siteapi.NewOptName(" ")},
			siteapi.SiteIntegrationsUpdateParams{WorkspaceId: acme, ID: id})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
		_, err = c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Config: siteapi.NewOptIntegrationConfigInput(smtpInput("", "p"))},
			siteapi.SiteIntegrationsUpdateParams{WorkspaceId: acme, ID: id})
		testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity, "an invalid config is rejected")

		stored, err := env.DB.Integration.Get(ctx, int64(id))
		require.NoError(t, err)
		assert.Equal(t, fixtures.IntegrationAcmeGenericAName, stored.Name, "rejected updates changed nothing")

		// Enabled toggles and demoting a non-default keep working.
		r6, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
			Enabled: siteapi.NewOptBool(false), IsDefault: siteapi.NewOptBool(false),
		}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: acme, ID: id})
		require.NoError(t, err)
		assert.False(t, r6.Enabled)
	})

	t.Run("archive", func(t *testing.T) {
		err := c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: foreign, ID: id})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)
		err = c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: acme, ID: idStr(99999)})
		testhelper.SiteRefusal(t, err, http.StatusNotFound)

		err = c.SiteIntegrationsDelete(ctx, siteapi.SiteIntegrationsDeleteParams{WorkspaceId: acme, ID: id})
		require.NoError(t, err)
		archived, err := env.DB.Integration.Query().Where(integration.ID(int64(id)), integration.DiscardedAtNotNil()).Exist(ctx)
		require.NoError(t, err)
		assert.True(t, archived, "archived, not removed")
	})
}
