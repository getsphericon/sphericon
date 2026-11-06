package automations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/automationsteps"
	"github.com/getsphericon/sphericon/internal/domain/sending/outbound"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A draft's step may name a disabled Integration, but Publish arms every send step, so it is
// refused while the Send route of any of them cannot send, though the other steps' routes can: the
// live version and the status stay as they were.
func TestPublishIsRefusedWhileAnyStepsIntegrationCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())

	ses := int64(fixtures.IntegrationAcmeSesID)
	edited, err := m.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
		Steps: &[]automationsteps.Step{emailVia(nil), {Type: automationsteps.Wait, Seconds: 60}, emailVia(&ses)},
	})
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
	mailgun := int64(fixtures.IntegrationAcmeMailgunID)
	_, err := m.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Steps: &[]automationsteps.Step{emailVia(&mailgun)}})
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

// An Automation with no send step sends through no Integration, so no route is asked and nothing
// stops its Publish.
func TestPublishWithoutASendStepDoesNotAskTheRoute(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := automations.New(env.Cipher, env.Bus, testhelper.Renderer(), env.OutboundModule())
	_, err := m.Update(ctx, s, fixtures.AutomationHoldDemoID, automations.UpdateInput{
		Steps: &[]automationsteps.Step{{Type: automationsteps.Wait, Seconds: 60}},
	})
	require.NoError(t, err)

	published, err := m.Publish(ctx, s, fixtures.AutomationHoldDemoID)

	require.NoError(t, err)
	_, draft := automations.LiveAndDraft(published)
	assert.Nil(t, draft, "the draft went live")
}
