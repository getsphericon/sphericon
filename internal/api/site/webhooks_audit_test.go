package site_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// audit.entry is selectable on a Webhook endpoint only with an Enterprise license
// (ADR 0022); without one the selection is rejected on create and update.
func TestWebhookEndpointSelectsAuditEntryOnlyUnderLicense(t *testing.T) {
	ctx := context.Background()
	create := func(env *testhelper.TestEnv) error {
		_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWebhooksCreate(ctx,
			&siteapi.CreateWebhookInput{URL: "https://x.test/audit", EventTypes: []string{"audit.entry"}},
			siteapi.SiteWebhooksCreateParams{WorkspaceId: fixtures.AcmePublicID})
		return err
	}

	require.NoError(t, create(testhelper.Setup(t)))
	testhelper.SiteRefusal(t, create(testhelper.Setup(t, testhelper.WithoutLicense())), http.StatusUnprocessableEntity)

	env := testhelper.Setup(t, testhelper.WithoutLicense())
	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteWebhooksUpdate(ctx,
		&siteapi.UpdateWebhookInput{EventTypes: []string{"audit.entry"}},
		siteapi.SiteWebhooksUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.WebhookCodebasicsID)})
	testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity)
}
