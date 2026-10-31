package external_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Exit and goal events go into the draft of a new Automation (#751); a goal that is the trigger,
// or that is also an exit event, is a 422.
func TestExternalAutomationExitAndGoalEvents(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:read", "automations:write")
	ctx := context.Background()

	res, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Upsell", TriggerEvent: "plan_limit_reached",
		ExitEvents: []string{"account_closed"}, GoalEvent: externalapi.NewOptNilString("subscription_expanded"),
	})
	require.NoError(t, err)
	created, ok := res.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, []string{"account_closed"}, created.Draft.Value.ExitEvents)
	assert.Equal(t, "subscription_expanded", created.Draft.Value.GoalEvent.Or(""))

	sameAsTrigger, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Bad", TriggerEvent: "plan_limit_reached", GoalEvent: externalapi.NewOptNilString("plan_limit_reached"),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsCreateUnprocessableEntity{}, sameAsTrigger)

	alsoAnExit, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{
		GoalEvent: externalapi.NewOptNilString("account_closed"),
	}, externalapi.AutomationsUpdateParams{ID: created.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AutomationsUpdateUnprocessableEntity{}, alsoAnExit)
}

// On a published Automation an edit of the exit and goal events writes the draft (ADR 0065): the
// live version, which new Enrollments begin on, keeps its own until a Publish.
func TestExternalAutomationEndingsEditOfAPublishedAutomationWritesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "automations:read", "automations:write")
	ctx := context.Background()

	res, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{
		ExitEvents: []string{"card_removed"}, GoalEvent: externalapi.NewOptNilString("invoice_paid"),
	}, externalapi.AutomationsUpdateParams{ID: entityIDString(fixtures.AutomationPaymentRecoveryID)})
	require.NoError(t, err)

	edited, ok := res.(*externalapi.AutomationResource)
	require.Truef(t, ok, "got %T", res)
	live, draft := edited.Live.Value, edited.Draft.Value
	assert.Equal(t, []string{"account_closed", "plan_downgraded", "email.clicked"}, live.ExitEvents)
	assert.Equal(t, "payment_succeeded", live.GoalEvent.Or(""))
	assert.Equal(t, []string{"card_removed"}, draft.ExitEvents)
	assert.Equal(t, "invoice_paid", draft.GoalEvent.Or(""))
}
