package external_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Exit and goal events go into the draft of a new Automation (#751); a goal that is the trigger,
// or that is also an exit event, is a 422.
func TestExternalAutomationExitAndGoalEvents(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	ctx := context.Background()

	created, err := c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Upsell", TriggerEvent: "plan_limit_reached",
		ExitEvents: []string{"account_closed"}, GoalEvent: externalapi.NewOptNilString("subscription_expanded"),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"account_closed"}, created.Draft.Value.ExitEvents)
	assert.Equal(t, "subscription_expanded", created.Draft.Value.GoalEvent.Or(""))

	_, err = c.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: "Bad", TriggerEvent: "plan_limit_reached", GoalEvent: externalapi.NewOptNilString("plan_limit_reached"),
	})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	_, err = c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(created.ID))),
		GoalEvent: externalapi.NewOptNilString("account_closed"),
	}, externalapi.AutomationsUpdateParams{ID: created.ID})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}

// On a published Automation an edit of the exit and goal events writes the draft (ADR 0065): the
// live version, which new Enrollments begin on, keeps its own until a Publish.
func TestExternalAutomationEndingsEditOfAPublishedAutomationWritesTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeAutomationsRead, apitokens.ScopeAutomationsWrite)
	ctx := context.Background()

	res, err := c.AutomationsUpdate(ctx, &externalapi.UpdateAutomationInput{Revision: externalapi.Revision(env.AutomationRevision(ctx, t, int64(entityIDString(fixtures.AutomationPaymentRecoveryID)))),
		ExitEvents: []string{"card_removed"}, GoalEvent: externalapi.NewOptNilString("invoice_paid"),
	}, externalapi.AutomationsUpdateParams{ID: entityIDString(fixtures.AutomationPaymentRecoveryID)})
	require.NoError(t, err)

	edited := res
	live, draft := edited.Live.Value, edited.Draft.Value
	assert.Equal(t, []string{"account_closed", "plan_downgraded", systemevent.EmailClicked}, live.ExitEvents)
	assert.Equal(t, "payment_succeeded", live.GoalEvent.Or(""))
	assert.Equal(t, []string{"card_removed"}, draft.ExitEvents)
	assert.Equal(t, "invoice_paid", draft.GoalEvent.Or(""))
}
