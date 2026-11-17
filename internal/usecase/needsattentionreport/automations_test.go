package needsattentionreport_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/integrationproviderstate"
	"github.com/getsphericon/sphericon/internal/domain/identity/suspension"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/holdreason"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/needsattentionreport"
)

// cannotSendOf is the automationCannotSend item about the Automation id, and whether there is one.
func cannotSendOf(items []needsattentionreport.Item, id int64) (needsattentionreport.Item, bool) {
	return itemAbout(items, needsattentionreport.KindAutomationCannotSend, id)
}

// Fixtures: the Jobs emails-then-tag Automation is active; its first email step goes through the
// Workspace default, its second through the Mailgun relay. This test archives the relay: one held
// step is enough.
func TestAnActiveAutomationWithAStepWhoseIntegrationIsArchivedCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	acme := env.DB.Scoped(fixtures.AcmeID)
	_, before := cannotSendOf(itemsOf(t, env, fixtures.AcmeID), fixtures.AutomationJobsEmailsThenTagID)
	require.NoError(t, acme.Integration().UpdateOneID(fixtures.IntegrationAcmeMailgunID).
		SetDiscardedAt(time.Now()).Exec(t.Context()))

	items := itemsOf(t, env, fixtures.AcmeID)

	assert.False(t, before, "it sends while its Integration is kept")
	held, ok := cannotSendOf(items, fixtures.AutomationJobsEmailsThenTagID)
	require.True(t, ok)
	assert.Equal(t, needsattentionreport.SeverityBlocked, held.Severity)
	assert.Equal(t, &needsattentionreport.Subject{ID: fixtures.AutomationJobsEmailsThenTagID, Name: fixtures.AutomationJobsEmailsThenTagName}, held.Subject)
	assert.Equal(t, new(holdreason.NoIntegration), held.HoldReason)
	assert.Nil(t, held.Since, "no moment is stored for when a route started holding")
	_, welcome := cannotSendOf(items, fixtures.AutomationWelcomeSeriesID)
	assert.False(t, welcome, "the Automations on the default still send")
}

// Fixtures: the Workspace default's sender sends from hello@codebasics.dev.
func TestAnActiveAutomationWhoseFromDomainIsNotVerifiedCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.Scoped(fixtures.AcmeID).SendingDomain().UpdateOneID(fixtures.SendingDomainCodebasicsID).
		SetVerificationLostAt(time.Now()).Exec(t.Context()))

	items := itemsOf(t, env, fixtures.AcmeID)

	held, ok := cannotSendOf(items, fixtures.AutomationWelcomeSeriesID)
	require.True(t, ok)
	assert.Equal(t, new(holdreason.UnverifiedDomain), held.HoldReason)
	_, holdDemo := cannotSendOf(items, fixtures.AutomationHoldDemoID)
	assert.True(t, holdDemo, "every Automation that cannot send is an item of its own")
}

func TestAnActiveAutomationWhoseProviderStoppedTheAccountCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.IntegrationProviderState.Update().Where(integrationproviderstate.IntegrationID(fixtures.IntegrationAcmeDefaultID)).
		SetProviderEnforcement(providers.EnforcementShutdown).Exec(t.Context()))

	items := itemsOf(t, env, fixtures.AcmeID)

	held, ok := cannotSendOf(items, fixtures.AutomationWelcomeSeriesID)
	require.True(t, ok)
	assert.Equal(t, new(holdreason.ProviderStopped), held.HoldReason)
}

// Fixtures: the provider stopped the Integration each of Meridian's Automations below sends through,
// so each Automation left out is left out for what it is.
func TestOnlyAnActiveAutomationThatSendsEmailInAWorkspaceThatSendsIsAnItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.MeridianID)

	_, paused := cannotSendOf(items, fixtures.AutomationMeridianInactiveID)
	assert.False(t, paused, "a paused Automation sends nothing")
	_, archived := cannotSendOf(items, fixtures.AutomationMeridianArchivedID)
	assert.False(t, archived, "an archived Automation sends nothing")
	_, tags := cannotSendOf(items, fixtures.AutomationMeridianNoEmailID)
	assert.False(t, tags, "an Automation with no email step sends through no Integration")
	_, stopped := cannotSendOf(items, fixtures.AutomationMeridianStoppedID)
	assert.True(t, stopped, "an active one with an email step is still an item")
}

// Fixtures: Meridian's draft-only Automation has a live version with a wait step and a draft with an
// email step through an Integration its provider stopped.
func TestAnEmailStepOnlyInTheDraftMakesNoItem(t *testing.T) {
	env := testhelper.Setup(t)

	items := itemsOf(t, env, fixtures.MeridianID)

	_, draft := cannotSendOf(items, fixtures.AutomationMeridianDraftOnlyID)
	assert.False(t, draft, "a draft sends nothing until it is published")
}

// The default is stopped too, so the suspension is what leaves the item out.
func TestASuspendedWorkspaceShowsNoAutomationThatCannotSend(t *testing.T) {
	env := testhelper.Setup(t)
	require.NoError(t, env.DB.IntegrationProviderState.Update().Where(integrationproviderstate.IntegrationID(fixtures.IntegrationAcmeDefaultID)).
		SetProviderEnforcement(providers.EnforcementShutdown).Exec(t.Context()))
	suspended, err := suspension.NewSuspender(env.Bus, time.Now).Suspend(t.Context(), fixtures.AcmeID, suspension.SuspendInput{By: suspension.System, Reason: "abuse"})
	require.NoError(t, err)
	require.True(t, suspended)

	items := itemsOf(t, env, fixtures.AcmeID)

	assert.NotContains(t, kinds(items), needsattentionreport.KindAutomationCannotSend, "the suspension is the layout's banner")
}
