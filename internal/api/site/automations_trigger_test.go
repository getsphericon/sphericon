package site_test

import (
	"encoding/json"
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

// An Event filter as a client writes it: every property test, nested and negated.
const eventFilterOfEveryTest = `{"kind":"group","combinator":"and","entries":[
	{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"paid"}},
	{"kind":"group","combinator":"or","not":true,"entries":[
		{"kind":"property","key":"cart.total","test":{"kind":"number","operator":"lt","value":10}},
		{"kind":"property","key":"coupon","test":{"kind":"presence","operator":"set"}}
	]}
]}`

// A version's Trigger is a union by kind (ADR 0133): an Event matcher is stored as sent and read
// back as {kind: "event", action, filter}, on create and on update, and without a filter it has
// none.
func TestSiteTriggerIsReadBackAsAnEventMatcher(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	filter := testhelper.ConditionInput[siteapi.EventFilterInput](t, eventFilterOfEveryTest)

	created, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: "Paid orders", Trigger: testhelper.SiteTrigger("order.completed", filter)},
		siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	read, err := c.SiteAutomationsGet(ctx, siteapi.SiteAutomationsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	unfiltered, err := c.SiteAutomationsUpdate(ctx, &siteapi.UpdateAutomationInput{Revision: read.Revision,
		Trigger: siteapi.NewOptAutomationTriggerInput(testhelper.SiteTrigger("order.refunded"))},
		siteapi.SiteAutomationsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)

	want := `{"kind":"event","action":"order.completed","filter":` + eventFilterOfEveryTest + `,"entryPolicy":"once","cooldown":3600}`
	assert.JSONEq(t, want, testhelper.JSONOf(t, &created.Draft.Value.Trigger))
	assert.JSONEq(t, want, testhelper.JSONOf(t, &read.Draft.Value.Trigger))
	assert.JSONEq(t, `{"kind":"event","action":"order.refunded","entryPolicy":"once","cooldown":3600}`, testhelper.JSONOf(t, &unfiltered.Draft.Value.Trigger))
}

// A filter says which Event, never which Contact: a test of the Contact has no value in the
// filter's type, so the body is refused with a 400 naming the leaf's path, and so is a property
// test that names a subject or a field the Trigger does not declare (ADR 0148, ADR 0181).
func TestSiteTriggerFilterRefusesWhatAFilterDoesNotHoldByItsPath(t *testing.T) {
	for _, tc := range []struct {
		name, leaf, path string
	}{
		{"a test of a Contact field", `{"kind":"attribute","attribute":"email","test":{"kind":"text","operator":"eq","value":"x"}}`, "trigger.filter.entries.1.entries.0"},
		{"a test of the Contact's Tag", `{"kind":"tag","name":"vip","operator":"has"}`, "trigger.filter.entries.1.entries.0"},
		{"a property test with a subject", `{"kind":"property","of":{"kind":"trigger"},"key":"url","test":{"kind":"text","operator":"eq","value":"x"}}`, "trigger.filter.entries.1.entries.0.of"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
				body["trigger"].(map[string]any)["filter"] = filterNesting(t, tc.leaf)
			}))

			_, err := edited.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{Name: "Misplaced", Trigger: testhelper.SiteTrigger("order.completed")},
				siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})

			refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}

	t.Run("an unknown field of the Trigger", func(t *testing.T) {
		env := testhelper.Setup(t)
		edited := env.SiteActorVia(t, fixtures.OwnerJohnEmail, testhelper.EditBody(func(body map[string]any) {
			body["trigger"].(map[string]any)["event"] = "order.completed"
		}))

		_, err := edited.SiteAutomationsCreate(t.Context(), &siteapi.CreateAutomationInput{Name: "Typo", Trigger: testhelper.SiteTrigger("order.completed")},
			siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})

		refused := testhelper.SiteRefusal(t, err, http.StatusBadRequest)
		assert.Equal(t, []string{unknownProperty}, refused.Errors.Or(nil)["trigger.event"])
	})
}

// A filter that names a definition names one of the catalogue: a Tag of contact.tagged or a
// Custom field of contact.field_changed the Workspace lacks is refused, and one it has is saved.
func TestSiteTriggerFilterRefusesAnUnknownTagOrCustomField(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	on := func(action, key, value string) siteapi.AutomationTriggerInput {
		return testhelper.SiteTrigger(action, testhelper.ConditionInput[siteapi.EventFilterInput](t,
			`{"kind":"group","combinator":"and","entries":[{"kind":"property","key":"`+key+`","test":{"kind":"text","operator":"eq","value":"`+value+`"}}]}`))
	}
	create := func(trigger siteapi.AutomationTriggerInput) error {
		_, err := c.SiteAutomationsCreate(ctx, &siteapi.CreateAutomationInput{Name: "Named", Trigger: trigger},
			siteapi.SiteAutomationsCreateParams{WorkspaceId: fixtures.AcmePublicID})
		return err
	}

	unknownTag := create(on(systemevent.ContactTagged, "tag", "no-such-tag"))
	unknownField := create(on(systemevent.ContactFieldChanged, "key", "no_such_field"))
	knownTag := create(on(systemevent.ContactUntagged, "tag", "vip"))
	knownField := create(on(systemevent.ContactFieldChanged, "key", "plan"))

	assert.Contains(t, testhelper.SiteRefusal(t, unknownTag, http.StatusUnprocessableEntity).Detail.Or(""), `"no-such-tag"`)
	assert.Contains(t, testhelper.SiteRefusal(t, unknownField, http.StatusUnprocessableEntity).Detail.Or(""), `"no_such_field"`)
	assert.NoError(t, knownTag)
	assert.NoError(t, knownField)
}

// filterNesting is an Event filter holding leaf at entries.1.entries.0, behind a valid test.
func filterNesting(t *testing.T, leaf string) map[string]any {
	t.Helper()
	var filter map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"kind":"group","combinator":"and","entries":[
		{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"paid"}},
		{"kind":"group","combinator":"or","entries":[`+leaf+`]}
	]}`), &filter))
	return filter
}
