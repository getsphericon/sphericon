package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// tagIsVip is the Event filter whose one test is that the Event's tag is vip.
const tagIsVip = `{"kind":"group","combinator":"and","entries":[{"kind":"property","key":"tag","test":{"kind":"text","operator":"eq","value":"vip"}}]}`

// Exit and goal events are Event matchers (ADR 0133) that go into the draft of a new Automation
// (#751) and are read back as written; the goal may share the Trigger's action.
func TestExternalAutomationExitAndGoalEvents(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	ctx := context.Background()
	vip := testhelper.ConditionInput[externalapi.EventFilterInput](t, tagIsVip)

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Upsell", Trigger: testhelper.ExternalTrigger(systemevent.ContactTagged),
		ExitEvents: []externalapi.EventMatcherInput{testhelper.ExternalMatcher("account_closed")},
		GoalEvent:  externalapi.NewOptNilEventMatcherInput(testhelper.ExternalMatcher(systemevent.ContactTagged, vip)),
	})
	require.NoError(t, err)

	assert.JSONEq(t, `[{"action":"account_closed"}]`, testhelper.JSONOfEach(t, created.Draft.Value.ExitEvents))
	assert.JSONEq(t, `{"action":"`+systemevent.ContactTagged+`","filter":`+tagIsVip+`}`, testhelper.JSONOf(t, &created.Draft.Value.GoalEvent))
}

// On a published Automation an edit of the exit and goal events writes the draft (ADR 0065): the
// live version, which new Enrollments begin on, keeps its own until a Publish.
func TestExternalAutomationEndingsEditOfAPublishedAutomationWritesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	ctx := context.Background()

	res, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(entityIDString(fixtures.AutomationPaymentRecoveryID)))),
		ExitEvents: []externalapi.EventMatcherInput{testhelper.ExternalMatcher("card_removed")},
		GoalEvent:  externalapi.NewOptNilEventMatcherInput(testhelper.ExternalMatcher("invoice_paid")),
	}, externalapi.AutomationsUpdateParams{ID: entityIDString(fixtures.AutomationPaymentRecoveryID)})
	require.NoError(t, err)

	live, draft := res.Live.Value, res.Draft.Value
	assert.JSONEq(t, `[{"action":"account_closed"},{"action":"plan_downgraded"},{"action":"`+systemevent.EmailClicked+`"}]`, testhelper.JSONOfEach(t, live.ExitEvents))
	assert.JSONEq(t, `{"action":"payment_succeeded"}`, testhelper.JSONOf(t, &live.GoalEvent))
	assert.JSONEq(t, `[{"action":"card_removed"}]`, testhelper.JSONOfEach(t, draft.ExitEvents))
	assert.JSONEq(t, `{"action":"invoice_paid"}`, testhelper.JSONOf(t, &draft.GoalEvent))
}
