package site_test

import (
	"context"
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

	created, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{
		Name: "Primary SMTP", Config: smtpInput("smtp.acme.test", "pw"),
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: acme})
	require.NoError(t, err)
	row := created.(*siteapi.SiteIntegrationResource)
	total, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Count(ctx)
	require.NoError(t, err)

	t.Run("list is scoped and requires a workspace", func(t *testing.T) {
		res, err := c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: acme})
		require.NoError(t, err)
		list, ok := res.(*siteapi.SiteIntegrationsListOKApplicationJSON)
		require.Truef(t, ok, "got %T", res)
		require.Len(t, *list, total, "the Workspace's integrations, the new one among them")
		assert.Equal(t, row.ID, (*list)[total-1].ID, "ordered by id")

		nf, err := c.SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: foreign})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsListNotFound{}, nf)

		_, err = env.SiteAnonymous(t).SiteIntegrationsList(ctx, siteapi.SiteIntegrationsListParams{WorkspaceId: acme})
		require.Error(t, err)
	})

	t.Run("create validates", func(t *testing.T) {
		r1, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{Name: "x", Config: smtpInput("h", "p")},
			siteapi.SiteIntegrationsCreateParams{WorkspaceId: foreign})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsCreateNotFound{}, r1)
		r2, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{Name: "   ", Config: smtpInput("h", "p")},
			siteapi.SiteIntegrationsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsCreateUnprocessableEntity{}, r2)
		r3, err := c.SiteIntegrationsCreate(ctx, &siteapi.CreateIntegrationInput{Name: "bad", Config: smtpInput("", "p")},
			siteapi.SiteIntegrationsCreateParams{WorkspaceId: acme})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsCreateUnprocessableEntity{}, r3, "an SMTP config without a host is rejected")
		n, err := env.DB.Scoped(fixtures.AcmeID).Integration().Query().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, total, n, "rejected creates persisted nothing")
	})

	t.Run("get", func(t *testing.T) {
		r1, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: foreign, ID: row.ID})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsGetNotFound{}, r1)
		r3, err := c.SiteIntegrationsGet(ctx, siteapi.SiteIntegrationsGetParams{WorkspaceId: acme, ID: idStr(99999)})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsGetNotFound{}, r3)
	})

	t.Run("update", func(t *testing.T) {
		in := &siteapi.UpdateIntegrationInput{Name: siteapi.NewOptString("Renamed")}
		r1, err := c.SiteIntegrationsUpdate(ctx, in, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: foreign, ID: row.ID})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsUpdateNotFound{}, r1)
		r3, err := c.SiteIntegrationsUpdate(ctx, in, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: acme, ID: idStr(99999)})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsUpdateNotFound{}, r3)
		r4, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Name: siteapi.NewOptString(" ")},
			siteapi.SiteIntegrationsUpdateParams{WorkspaceId: acme, ID: row.ID})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsUpdateUnprocessableEntity{}, r4)
		r5, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{Config: siteapi.NewOptIntegrationConfigInput(smtpInput("", "p"))},
			siteapi.SiteIntegrationsUpdateParams{WorkspaceId: acme, ID: row.ID})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsUpdateUnprocessableEntity{}, r5, "an invalid config is rejected")

		stored, err := env.DB.Integration.Get(ctx, int64(row.ID))
		require.NoError(t, err)
		assert.Equal(t, "Primary SMTP", stored.Name, "rejected updates changed nothing")

		// Enabled toggles and demoting a non-default keep working.
		r6, err := c.SiteIntegrationsUpdate(ctx, &siteapi.UpdateIntegrationInput{
			Enabled: siteapi.NewOptBool(false), IsDefault: siteapi.NewOptBool(false),
		}, siteapi.SiteIntegrationsUpdateParams{WorkspaceId: acme, ID: row.ID})
		require.NoError(t, err)
		assert.False(t, r6.(*siteapi.SiteIntegrationResource).Enabled)
	})

	t.Run("archive", func(t *testing.T) {
		r1, err := c.SiteIntegrationsArchive(ctx, siteapi.SiteIntegrationsArchiveParams{WorkspaceId: foreign, ID: row.ID})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsArchiveNotFound{}, r1)
		r3, err := c.SiteIntegrationsArchive(ctx, siteapi.SiteIntegrationsArchiveParams{WorkspaceId: acme, ID: idStr(99999)})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationsArchiveNotFound{}, r3)

		r4, err := c.SiteIntegrationsArchive(ctx, siteapi.SiteIntegrationsArchiveParams{WorkspaceId: acme, ID: row.ID})
		require.NoError(t, err)
		assert.IsType(t, &siteapi.SiteIntegrationResource{}, r4)
		archived, err := env.DB.Integration.Query().Where(integration.ID(int64(row.ID)), integration.DiscardedAtNotNil()).Exist(ctx)
		require.NoError(t, err)
		assert.True(t, archived, "archived, not removed")
	})
}
