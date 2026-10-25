package jobs_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/ent/capturedmessage"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/stepping"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An Automation step sent through the Test inbox needs no verified Sending domain: the inbox's
// From is not on one, and the step is Sent and kept as a Captured message of the automation
// surface (ADR 0058).
func TestAnAutomationStepThroughTheTestInboxIsSentAndCapturedWithoutAVerifiedDomain(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	created, err := owner.SiteIntegrationsCreate(ctx, &siteapi.SiteCreateIntegrationInput{
		Name: "Test inbox",
		Config: siteapi.SiteIntegrationConfigInput{
			OneOf: siteapi.NewSiteInboxConfigInputSiteIntegrationConfigInputSum(siteapi.SiteInboxConfigInput{Kind: siteapi.SiteInboxConfigInputKindInbox}),
		},
	}, siteapi.SiteIntegrationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	inbox := created.(*siteapi.SiteIntegrationResource)
	chosen, err := owner.SiteIntegrationsUpdate(ctx, &siteapi.SiteUpdateIntegrationInput{IsDefault: siteapi.NewOptBool(true)},
		siteapi.SiteIntegrationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: inbox.ID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteIntegrationResource{}, chosen)
	liam := env.DB.Contact.GetX(ctx, fixtures.ContactLiamID)
	runID := enrollVia(t, env, fixtures.AutomationJobsSingleEmailID, liam.ID)

	env.RunQueuedSteps(t, stepping.New(env.Bus, env.OutboundModule(), noFence))

	assert.Equal(t, automationrun.StatusCompleted, env.DB.AutomationRun.GetX(ctx, runID).Status)
	kept := env.DB.CapturedMessage.Query().
		Where(capturedmessage.Recipient(*liam.Email), capturedmessage.SurfaceEQ(capturedmessage.SurfaceAutomation)).
		OnlyX(ctx)
	assert.True(t, kept.FromUnverified, "the From is not on a verified Sending domain, and it was sent all the same")
	assert.Equal(t, "Hi "+*liam.FirstName, kept.Subject)
	require.NotNil(t, kept.OutboundMessageID)
	sent := env.DB.OutboundMessage.GetX(ctx, *kept.OutboundMessageID)
	assert.Equal(t, outboundmessage.StatusSent, sent.Status)
	assert.True(t, sent.Captured)
}
