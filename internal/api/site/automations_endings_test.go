package site_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// statusIs is the Event filter whose one test is that the Event's status is value.
func statusIs(value string) string {
	return `{"kind":"group","combinator":"and","entries":[{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"` + value + `"}}]}`
}

// Exit and goal events are Event matchers (ADR 0133), read on each version and written to the
// draft (#751, ADR 0065): the live version keeps its own, an explicit null removes the goal, an
// omitted list is kept, and an exit and the goal may share an action with each other and with the
// Trigger.
func TestSiteAutomationExitAndGoalEventsWriteTheDraft(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := t.Context()
	params := siteapi.SiteAutomationsUpdateParams{WorkspaceId: siteapi.WorkspaceId(fixtures.AcmePublicID), ID: idStr(fixtures.AutomationPaymentRecoveryID)}
	cancelled := testhelper.ConditionInput[siteapi.EventFilterInput](t, statusIs("cancelled"))
	paid := testhelper.ConditionInput[siteapi.EventFilterInput](t, statusIs("paid"))

	edited, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(params.ID))),
		ExitEvents: []siteapi.EventMatcherInput{testhelper.SiteMatcher("payment_failed"), testhelper.SiteMatcher("order.closed", cancelled)},
		GoalEvent:  siteapi.NewOptNilEventMatcherInput(testhelper.SiteMatcher("order.closed", paid)),
	}, params)
	require.NoError(t, err)
	noGoal := siteapi.OptNilEventMatcherInput{}
	noGoal.SetToNull()
	cleared, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: siteapi.Revision(env.AutomationRevision(ctx, t, int64(params.ID))), GoalEvent: noGoal}, params)
	require.NoError(t, err)

	assert.JSONEq(t, `[{"action":"account_closed"},{"action":"plan_downgraded"},{"action":"`+systemevent.EmailClicked+`"}]`,
		testhelper.JSONOfEach(t, edited.Live.Value.ExitEvents))
	assert.JSONEq(t, `{"action":"payment_succeeded"}`, testhelper.JSONOf(t, &edited.Live.Value.GoalEvent))
	assert.JSONEq(t, `[{"action":"payment_failed"},{"action":"order.closed","filter":`+statusIs("cancelled")+`}]`,
		testhelper.JSONOfEach(t, edited.Draft.Value.ExitEvents))
	assert.JSONEq(t, `{"action":"order.closed","filter":`+statusIs("paid")+`}`, testhelper.JSONOf(t, &edited.Draft.Value.GoalEvent))
	assert.True(t, cleared.Draft.Value.GoalEvent.Null, "null removes the goal event")
	assert.Len(t, cleared.Draft.Value.ExitEvents, 2, "an omitted list is kept")
}

// An exit's or the goal's filter says which Event, never which Contact: a test of the Contact is
// refused with a 400 naming the leaf's path (ADR 0148, ADR 0181).
func TestSiteEndingFilterRefusesATestOfTheContactByItsPath(t *testing.T) {
	const leaf = `{"kind":"tag","name":"vip","operator":"has"}`
	for _, tc := range []struct {
		name string
		edit func(body map[string]any, filter map[string]any)
		path string
	}{
		{"in an exit", func(body, filter map[string]any) {
			body["exitEvents"] = []any{map[string]any{"action": "order.closed", "filter": filter}}
		}, "exitEvents.0.filter.entries.1.entries.0"},
		{"in the goal", func(body, filter map[string]any) {
			body["goalEvent"] = map[string]any{"action": "order.closed", "filter": filter}
		}, "goalEvent.filter.entries.1.entries.0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
				tc.edit(body, filterNesting(t, leaf))
			}))

			_, err := edited.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{Name: "Misplaced", Trigger: testhelper.SiteTrigger("order.placed")},
				siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})

			refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}

// A goal's filter that names a Tag the Workspace lacks is refused: it would never match.
func TestSiteGoalFilterRefusesAnUnknownTag(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	unknown := testhelper.ConditionInput[siteapi.EventFilterInput](t,
		`{"kind":"group","combinator":"and","entries":[{"kind":"property","key":"tag","test":{"kind":"text","operator":"eq","value":"no-such-tag"}}]}`)

	_, err := c.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{Name: "Named", Trigger: testhelper.SiteTrigger("order.placed"),
		GoalEvent: siteapi.NewOptNilEventMatcherInput(testhelper.SiteMatcher(systemevent.ContactTagged, unknown))},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})

	assert.Contains(t, testhelper.SiteRefusal(t, err, http.StatusUnprocessableEntity).Detail.Or(""), `"no-such-tag"`)
}

// An exit or goal event is a closed object (ADR 0148): a property the contract does not declare is
// a 400 naming its path.
func TestSiteEndingRefusesAPropertyTheContractDoesNotDeclare(t *testing.T) {
	env := testhelper.Setup(t)
	edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
		body["exitEvents"] = []any{map[string]any{"action": "order.closed", "kind": "event"}}
	}))

	_, err := edited.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{Name: "Typo", Trigger: testhelper.SiteTrigger("order.placed")},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})

	refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
	assert.Equal(t, []string{unknownProperty}, refused.Errors.Or(nil)["exitEvents.0.kind"])
}
