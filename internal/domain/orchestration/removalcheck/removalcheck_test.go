package removalcheck_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/customfield"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/liveuse"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// underLock runs check in a transaction of the Workspace holding its lock (ADR 0099, ADR 0108), as
// every removal that checks a row runs it, and returns what it answered.
func underLock(t *testing.T, env *testhelper.TestEnv, workspaceID int64, check func(*ent.WorkspaceLocked) error) error {
	t.Helper()
	_, answer := testhelper.UnderWorkspaceLock(t, env.Bus, env.DB.Scoped(workspaceID), func(l *ent.WorkspaceLocked) (struct{}, error) {
		return struct{}{}, check(l)
	})
	return answer
}

// A Tag is in Definition use through a Segment's Condition, a step of a kept Automation and a kept
// Rule (in a Condition leaf or a tag Action), each named in that order and by id order.
func TestATagNamedByDefinitionsIsRefusedNamingThemInOrder(t *testing.T) {
	env := testhelper.Setup(t)

	vip := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error { return env.RemovalCheck.Tag(t.Context(), s, fixtures.TagVipName) })
	newsletter := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.Tag(t.Context(), s, fixtures.TagNewsletterName)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectTag, Users: []liveuse.User{
		{Kind: liveuse.Automation, ID: fixtures.AutomationTagOnEngagementID, Name: fixtures.AutomationTagOnEngagementName},
		{Kind: liveuse.Rule, ID: fixtures.RuleVipOnlyID, Name: fixtures.RuleVipOnlyName},
		{Kind: liveuse.Rule, ID: fixtures.RuleMultiTriggerID, Name: fixtures.RuleMultiTriggerName},
	}}, vip)
	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectTag, Users: []liveuse.User{
		{Kind: liveuse.Segment, ID: fixtures.SegmentNewsletterReadersID, Name: fixtures.SegmentNewsletterReadersName},
	}}, newsletter)
}

// The Condition an armed Broadcast copied from its Segment keeps the Tag's name, so it holds the
// Tag on its own (ADR 0065).
func TestATagNamedOnlyInAnArmedBroadcastsCopyIsRefused(t *testing.T) {
	env := testhelper.Setup(t)

	refused := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.Tag(t.Context(), s, fixtures.TagRenewalDueName)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectTag, Users: []liveuse.User{
		{Kind: liveuse.Broadcast, ID: fixtures.BroadcastScheduledRenewalID, Name: fixtures.BroadcastScheduledRenewalName},
	}}, refused)
}

func TestATagNothingNamesPasses(t *testing.T) {
	env := testhelper.Setup(t)

	answer := underLock(t, env, fixtures.AcmeID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.Tag(t.Context(), s, fixtures.TagUnusedName)
	})

	assert.NoError(t, answer)
}

// A retype is refused while a Segment tests the field with a leaf the new type does not take; a
// leaf every type takes (notNull) does not hold the type. Duff's "Ends soon" tests ends_on with a
// date leaf, "Has it" only that has_it is there.
func TestACustomFieldRetypeIsRefusedOnlyByALeafTheNewTypeDoesNotTake(t *testing.T) {
	env := testhelper.Setup(t)

	endsOn := underLock(t, env, fixtures.DuffID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.CustomFieldRetype(t.Context(), s, fixtures.CustomFieldDuffEndsOnKey, customfield.TypeString)
	})
	hasIt := underLock(t, env, fixtures.DuffID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.CustomFieldRetype(t.Context(), s, fixtures.CustomFieldDuffHasItKey, customfield.TypeString)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectCustomField, Users: []liveuse.User{
		{Kind: liveuse.Segment, ID: fixtures.SegmentDuffEndsSoonID, Name: fixtures.SegmentDuffEndsSoonName},
	}}, endsOn)
	assert.NoError(t, hasIt)
}

// The default Integration is in Live use through the scheduled and the sending Broadcast that name
// none and the active Automation that sends email: Broadcasts first, then Automations.
func TestAnIntegrationLiveWorkSendsThroughIsRefusedNamingItInOrder(t *testing.T) {
	env := testhelper.Setup(t)

	refused := underLock(t, env, fixtures.MeridianID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.Integration(t.Context(), s, fixtures.IntegrationMeridianDefaultID)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectIntegration, Users: []liveuse.User{
		{Kind: liveuse.Broadcast, ID: fixtures.BroadcastMeridianScheduledID, Name: fixtures.BroadcastMeridianScheduledName},
		{Kind: liveuse.Broadcast, ID: fixtures.BroadcastMeridianSendingID, Name: fixtures.BroadcastMeridianSendingName},
		{Kind: liveuse.Automation, ID: fixtures.AutomationMeridianWelcomeID, Name: fixtures.AutomationMeridianWelcomeName},
	}}, refused)
}

// A draft's email sends nothing until it is armed, so an Integration only a draft names passes.
func TestAnIntegrationOnlyADraftSendsThroughPasses(t *testing.T) {
	env := testhelper.Setup(t)

	answer := underLock(t, env, fixtures.MeridianID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.Integration(t.Context(), s, fixtures.IntegrationMeridianDraftOnlyID)
	})

	assert.NoError(t, answer)
}

// A Sending domain is used through the From a send's email goes out with: a Broadcast and an
// Automation step with no From of their own send from the From of the Integration they go through,
// read from the sender the resolver builds for it.
func TestASendingDomainIsRefusedThroughTheFromOfTheIntegrationASendGoesThrough(t *testing.T) {
	env := testhelper.Setup(t)

	refused := underLock(t, env, fixtures.MeridianID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.SendingDomain(t.Context(), s, fixtures.SendingDomainMeridianRelayDomain)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectSendingDomain, Users: []liveuse.User{
		{Kind: liveuse.Broadcast, ID: fixtures.BroadcastMeridianViaRelayID, Name: fixtures.BroadcastMeridianViaRelayName},
		{Kind: liveuse.Automation, ID: fixtures.AutomationMeridianViaRelayID, Name: fixtures.AutomationMeridianViaRelayName},
	}}, refused)
}

// A Broadcast's own From uses its domain; a failed Broadcast is history and holds none.
func TestASendingDomainIsRefusedThroughABroadcastsOwnFromAndNotByHistory(t *testing.T) {
	env := testhelper.Setup(t)

	news := underLock(t, env, fixtures.MeridianID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.SendingDomain(t.Context(), s, fixtures.SendingDomainMeridianNewsDomain)
	})
	mail := underLock(t, env, fixtures.MeridianID, func(s *ent.WorkspaceLocked) error {
		return env.RemovalCheck.SendingDomain(t.Context(), s, fixtures.SendingDomainMeridianMailDomain)
	})

	assert.Equal(t, &liveuse.Error{What: liveuse.SubjectSendingDomain, Users: []liveuse.User{
		{Kind: liveuse.Broadcast, ID: fixtures.BroadcastMeridianFromNewsID, Name: fixtures.BroadcastMeridianFromNewsName},
	}}, news)
	assert.NoError(t, mail)
}
