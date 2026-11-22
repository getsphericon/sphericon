package automations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Publish arms an email send step, whose footer carries the Workspace's postal address (ADR 0156):
// without one the Automation stays inactive with its draft. AutomationWinBack is inactive, its
// draft waits a week and then sends an email.
func TestPublishOfAnEmailStepIsRefusedWithoutAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	env.ClearPostalAddress(t, fixtures.AcmeID)

	_, err := m.Publish(t.Context(), s, fixtures.AutomationWinBackID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationWinBackID))

	require.ErrorIs(t, err, outboundmessages.ErrPostalAddressRequired)
	after, err := m.Get(t.Context(), s, fixtures.AutomationWinBackID)
	require.NoError(t, err)
	live, draft := automations.LiveAndDraft(after)
	assert.Equal(t, automation.StatusInactive, after.Status)
	assert.Nil(t, live, "nothing went live")
	assert.Equal(t, int64(fixtures.AutomationWinBackVersionID), draft.ID, "the draft stays")
}

// Making an inactive Automation active again arms its email step anew: without an address the
// resume is refused as well. AutomationHoldDemo is active with one email step.
func TestResumingAnEmailAutomationIsRefusedWithoutAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	_, err := m.Deactivate(t.Context(), s, fixtures.AutomationHoldDemoID)
	require.NoError(t, err)
	env.ClearPostalAddress(t, fixtures.AcmeID)

	_, err = m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID))

	require.ErrorIs(t, err, outboundmessages.ErrPostalAddressRequired)
	status := env.DB.Automation.GetX(t.Context(), fixtures.AutomationHoldDemoID).Status
	assert.Equal(t, automation.StatusInactive, status)
}

// The address is the marketing email footer's: an Automation whose only send step is a text is
// published without it.
func TestAnSMSOnlyAutomationIsPublishedWithoutAPostalAddress(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	twilio := int64(env.AcmeTwilio(t).Integration.ID)
	_, err := m.Update(t.Context(), s, fixtures.AutomationHoldDemoID, automations.UpdateInput{Revision: testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID),
		Steps: steps(automationstep.Wait{Seconds: 60}, smsVia(&twilio, "Hi")),
	})
	require.NoError(t, err)
	env.ClearPostalAddress(t, fixtures.AcmeID)

	published, err := m.Publish(t.Context(), s, fixtures.AutomationHoldDemoID, testhelper.AutomationRevisionIn(t.Context(), t, s, fixtures.AutomationHoldDemoID))

	require.NoError(t, err)
	assert.Equal(t, automation.StatusActive, published.Status)
}
