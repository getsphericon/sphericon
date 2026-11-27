package external_test

import (
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/rules"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Engagement tracking consent reaches Conditions and Rules (ADR 0120): a Condition field per Channel
// reads it, and its Events trigger a Rule like any System event.

// A Segment of the Contacts whose email refused tracking lists the Contact with a refused address,
// not one with a granted or an unset one; a Segment of those with none recorded lists the unset one.
func TestASegmentOnTheEmailTrackingConsentListsWhoRefused(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeSegmentsRead, apitokens.ScopeSegmentsWrite, apitokens.ScopeContactsWrite)
	_, err := c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, time.Time{}))
	require.NoError(t, err)
	_, err = c.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactBobEmail, externalapi.EngagementTrackingStateGranted, time.Time{}))
	require.NoError(t, err)

	refused := segmentMembers(t, env, c, "Refused", `{"kind":"engagementTracking","channel":"email","test":{"kind":"text","operator":"eq","value":"denied"}}`)
	const notSet = `{"kind":"engagementTracking","channel":"email","test":{"kind":"presence","operator":"notSet"}}`
	unset := segmentMembers(t, env, c, "Unset", notSet)
	liam := segmentMembers(t, env, c, "Liam unset", `{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"eq","value":"`+fixtures.ContactLiamEmail+`"}},`+notSet)

	assert.Equal(t, []string{fixtures.ContactAliceEmail}, refused)
	assert.Equal(t, []string{fixtures.ContactLiamEmail}, liam)
	assert.NotContains(t, unset, fixtures.ContactAliceEmail)
	assert.NotContains(t, unset, fixtures.ContactBobEmail)
}

// segmentMembers creates through /api the Segment name of the leaves (JSON objects joined by
// commas) and lists the emails of its Contacts as a member reads them.
func segmentMembers(t *testing.T, env *testhelper.TestEnv, c *externalapi.Client, name, leaves string) []string {
	t.Helper()
	segment, err := c.SegmentsCreate(t.Context(), &externalapi.CreateSegmentInput{
		Name:      externalapi.Name(name),
		Condition: testhelper.ConditionInput[externalapi.SegmentConditionInput](t, `{"kind":"group","combinator":"and","entries":[`+leaves+`]}`),
	})
	require.NoError(t, err)
	members, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteSegmentContactsList(t.Context(), siteapi.SiteSegmentContactsListParams{
		WorkspaceId: fixtures.AcmePublicID, SegmentId: siteapi.EntityId(segment.ID), Limit: siteapi.NewOptInt32(100),
	})
	require.NoError(t, err)
	return lo.Map(members.Items, func(item siteapi.SiteContactResource, _ int) string { return string(item.Email.Value) })
}

// A Rule triggered by email.engagement_tracking_denied fires when a refusal of a Contact's email is
// recorded through /api, and its Action applies the Tag.
func TestARuleOnARefusalOfTrackingFiresWhenOneIsRecorded(t *testing.T) {
	env := testhelper.Setup(t)
	author := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	publisher := env.ExternalScoped(t, apitokens.ScopeAutomationsPublish)
	recorder := env.ExternalScoped(t, apitokens.ScopeContactsWrite)
	rule, err := author.RulesCreate(t.Context(), &externalapi.CreateRuleInput{
		Name:          "Ask again later",
		TriggerEvents: []string{systemevent.EmailEngagementTrackingDenied},
		Actions:       []externalapi.RuleActionInput{{Type: externalapi.RuleActionTypeApplyTag, Tag: externalapi.NewOptString("refused-tracking")}},
	})
	require.NoError(t, err)
	_, err = publisher.RulesPublish(t.Context(), &externalapi.RevisionInput{Revision: externalapi.Revision(env.RuleRevision(t.Context(), t, int64(rule.ID)))},
		externalapi.RulesPublishParams{ID: rule.ID})
	require.NoError(t, err)

	_, err = recorder.EngagementTrackingCreate(t.Context(), trackingOf(fixtures.ContactAliceEmail, externalapi.EngagementTrackingStateDenied, time.Time{}))
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, jobs.EvaluateRulesArgs{}, rules.ActionsArgs{})

	alice := siteContact(t, env, fixtures.ContactAliceID)
	assert.Contains(t, alice.Tags, "refused-tracking")
}

// siteContact is the Contact as a member reads it on its card.
func siteContact(t *testing.T, env *testhelper.TestEnv, contactID int64) *siteapi.SiteContactResource {
	t.Helper()
	c, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteContactsGet(t.Context(), siteapi.SiteContactsGetParams{
		WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(contactID),
	})
	require.NoError(t, err)
	return c
}
