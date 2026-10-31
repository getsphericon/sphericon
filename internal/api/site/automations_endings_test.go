package site_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Exit and goal events are read on each version and written to the draft (#751, ADR 0065): the
// live version keeps its own, an explicit null removes the goal, and an exit equal to the trigger
// is a 422.
func TestSiteAutomationExitAndGoalEventsWriteTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	params := siteapi.SiteAutomationsUpdateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), ID: idStr(fixtures.AutomationPaymentRecoveryID)}

	res, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{
		ExitEvents: []string{"card_removed"},
		GoalEvent:  siteapi.NewOptNilString("invoice_paid"),
	}, params)
	require.NoError(t, err)
	edited, ok := res.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, []string{"account_closed", "plan_downgraded", "email.clicked"}, edited.Live.Value.ExitEvents)
	assert.Equal(t, "payment_succeeded", edited.Live.Value.GoalEvent.Or(""))
	assert.Equal(t, []string{"card_removed"}, edited.Draft.Value.ExitEvents)
	assert.Equal(t, "invoice_paid", edited.Draft.Value.GoalEvent.Or(""))

	noGoal := siteapi.OptNilString{}
	noGoal.SetToNull()
	res, err = c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{GoalEvent: noGoal}, params)
	require.NoError(t, err)
	cleared, ok := res.(*siteapi.SiteAutomationResource)
	require.Truef(t, ok, "got %T", res)
	assert.True(t, cleared.Draft.Value.GoalEvent.Null, "null removes the goal event")
	assert.Equal(t, []string{"card_removed"}, cleared.Draft.Value.ExitEvents, "an omitted list is kept")

	refused, err := c.SiteAutomationsUpdate(ctx, &siteapi.SiteUpdateAutomationInput{ExitEvents: []string{"payment_failed"}}, params)
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAutomationsUpdateUnprocessableEntity{}, refused)
}
