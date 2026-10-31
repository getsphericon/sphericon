package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A draft may name a disabled Integration, but Publish arms the email step, so it is refused while
// the Send route cannot send: the live version and the status stay as they were.
func TestPublishIsRefusedWhileTheDraftsIntegrationCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())

	edited, err := m.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{IntegrationID: patch.Set[int64](fixtures.IntegrationAcmeSesID)})
	require.NoError(t, err, "a draft may name a disabled Integration")
	_, draft := automations.LiveAndDraft(edited)
	require.NotNil(t, draft)

	_, err = m.Publish(ctx, s, fixtures.AutomationHoldDemoID)

	require.ErrorIs(t, err, outbound.ErrRouteHeld)
	after, err := m.Get(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	live, kept := automations.LiveAndDraft(after)
	assert.Equal(t, int64(fixtures.AutomationHoldDemoVersionID), live.ID, "the live version is untouched")
	assert.Equal(t, draft.ID, kept.ID, "the draft stays to be fixed")
}

// An Integration disabled after Publish does not stop the Automation (its sends are held), but
// making it active again arms it anew: Publish without a draft is refused as well.
func TestResumingAnAutomationIsRefusedOnceItsIntegrationCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	_, err := m.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{IntegrationID: patch.Set[int64](fixtures.IntegrationAcmeMailgunID)})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err, "the Integration can send when it is published")
	_, err = m.Deactivate(ctx, s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	env.DB.Integration.UpdateOneID(fixtures.IntegrationAcmeMailgunID).SetEnabled(false).ExecX(ctx)

	_, err = m.Publish(ctx, s, fixtures.AutomationHoldDemoID)

	require.ErrorIs(t, err, outbound.ErrRouteHeld)
	status := env.DB.Automation.GetX(ctx, fixtures.AutomationHoldDemoID).Status
	assert.Equal(t, automation.StatusInactive, status)
}

// An Automation with no email step sends through no Integration, so a route that cannot send does
// not stop its Publish.
func TestPublishWithoutAnEmailStepDoesNotAskTheRoute(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	_, err := m.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
		IntegrationID: patch.Set[int64](fixtures.IntegrationAcmeSesID),
		Steps:         &[]automations.Step{{Type: automations.StepWait, Seconds: 60}},
	})
	require.NoError(t, err)

	published, err := m.Publish(ctx, s, fixtures.AutomationHoldDemoID)

	require.NoError(t, err)
	_, draft := automations.LiveAndDraft(published)
	assert.Nil(t, draft, "the draft went live")
}
