package external_test

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The ids a Wait for event of these tests is written with: the Step, its paths, a Step of its
// arrived path, and a second Wait for event.
const (
	waitForEventID = "7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c01"
	arrivedPathID  = "7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c02"
	timedOutPathID = "7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c03"
	thankedID      = "7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c04"
	otherWaitID    = "7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c05"
)

// orderOver100 is the Event matcher of an order.created whose total is over 100, as a client writes
// it.
const orderOver100 = `{"action":"order.created","filter":{"kind":"group","combinator":"and","entries":[` +
	`{"kind":"property","key":"total","test":{"kind":"number","operator":"gt","value":100}}]}}`

// vipOrProCondition is a Wait for event's Condition: the Contact has the Tag vip, or began the
// Enrollment on a pro plan.
const vipOrProCondition = `{"kind":"group","combinator":"or","entries":[{"kind":"tag","name":"vip","operator":"has"},` +
	`{"kind":"property","of":{"kind":"trigger"},"key":"plan","test":{"kind":"text","operator":"eq","value":"pro"}}]}`

// purchaseWait is a Wait for event named name, with the id given, that waits three days for an
// order or a booked call, as a client writes it.
func purchaseWait(id, name string) string {
	return `{"id":"` + id + `","type":"wait_for_event","name":"` + name + `","timeoutSeconds":259200,` +
		`"awaits":{"kind":"event","matchers":[{"action":"order.created"},{"action":"call.booked"}]},"arrived":{"steps":[]},"timedOut":{"steps":[]}}`
}

// saveWaitForEvent creates an Automation whose root path holds the Wait for events written as
// docs, then a wait, ended by the exits given.
func saveWaitForEvent(t *testing.T, c *externalapi.Client, exits []externalapi.EventMatcherInput, docs ...string) (*externalapi.AutomationResource, error) {
	t.Helper()
	steps := make([]any, 0, len(docs)+1)
	for _, doc := range docs {
		steps = append(steps, testhelper.ExternalWaitForEvent(t, doc))
	}
	steps = append(steps, externalapi.WaitStepInput{Seconds: 60})
	return c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Waits for a purchase", Trigger: testhelper.ExternalTrigger("signup"), ExitEvents: exits,
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(steps...)),
	})
}

// editSteps writes the Steps of the Automation's draft, the Wait for events written as docs.
func editSteps(t *testing.T, c *externalapi.Client, a *externalapi.AutomationResource, docs ...string) (*externalapi.AutomationResource, error) {
	t.Helper()
	steps := make([]any, len(docs))
	for i, doc := range docs {
		steps[i] = testhelper.ExternalWaitForEvent(t, doc)
	}
	return c.AutomationsUpdate(t.Context(), &externalapi.UpdateAutomationInput{
		Revision: a.Revision, Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(steps...)),
	}, externalapi.AutomationsUpdateParams{ID: a.ID})
}

// A Wait for event round-trips through /api as {type: "wait_for_event", name, timeoutSeconds,
// awaits: {kind, matchers, condition}, arrived, timedOut}, its matchers with their filters, its paths
// with their ids and Steps, at the root and in a Split's path.
func TestExternalAutomationKeepsAWaitForEvent(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	written := `{"id":"` + waitForEventID + `","type":"wait_for_event","name":"purchase","timeoutSeconds":259200,` +
		`"awaits":{"kind":"event","matchers":[` + orderOver100 + `,{"action":"call.booked"}],"condition":` + vipOrProCondition + `},` +
		`"arrived":{"id":"` + arrivedPathID + `","steps":[{"id":"` + thankedID + `","type":"apply_tag","tag":"thanked"}]},` +
		`"timedOut":{"id":"` + timedOutPathID + `","steps":[]}}`
	nested := `{"type":"split","paths":[{"weight":50,"steps":[` + purchaseWait(otherWaitID, "booking") + `]},{"weight":50,"steps":[]}]}`
	created, err := c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Waits for a purchase", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(
			testhelper.ExternalWaitForEvent(t, written),
			testhelper.ExternalSplit(t, nested),
		)),
	})
	require.NoError(t, err)

	got, err := c.AutomationsGet(t.Context(), externalapi.AutomationsGetParams{ID: created.ID})

	require.NoError(t, err)
	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	require.Equal(t, externalapi.WaitForEventStepAutomationStepSum, steps[0].OneOf.Type)
	assert.JSONEq(t, written, testhelper.JSONOf(t, &steps[0].OneOf.WaitForEventStep))
	inSplit := steps[1].OneOf.SplitStep.Paths[0].Steps
	require.Len(t, inSplit, 1)
	require.Equal(t, externalapi.WaitForEventStepSplitPathStepSum, inSplit[0].OneOf.Type, "a Wait for event in a Split's path")
	assert.NotEmpty(t, inSplit[0].OneOf.WaitForEventStep.Arrived.ID, "its paths are given ids")
}

// A Wait for event that awaits an action an exit matches with no filter is refused, naming it: the
// exit would end the Enrollment first. With a filtered exit on that action it is accepted, and an
// edit that later adds the unfiltered exit is refused too.
func TestExternalAutomationRefusesAWaitForAnEventAnUnfilteredExitEndsOn(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	paid := externalapi.EventFilterInput{}
	require.NoError(t, paid.UnmarshalJSON([]byte(`{"kind":"group","combinator":"and","entries":[`+
		`{"kind":"property","key":"status","test":{"kind":"text","operator":"eq","value":"paid"}}]}`)))

	_, unfiltered := saveWaitForEvent(t, c, []externalapi.EventMatcherInput{testhelper.ExternalMatcher("order.created")}, purchaseWait(waitForEventID, "purchase"))
	filtered, err := saveWaitForEvent(t, c, []externalapi.EventMatcherInput{testhelper.ExternalMatcher("order.created", paid)}, purchaseWait(waitForEventID, "purchase"))
	require.NoError(t, err)
	_, exitAdded := c.AutomationsUpdate(t.Context(), &externalapi.UpdateAutomationInput{
		Revision: filtered.Revision, ExitEvents: []externalapi.EventMatcherInput{testhelper.ExternalMatcher("call.booked")},
	}, externalapi.AutomationsUpdateParams{ID: filtered.ID})

	detail := refusedNaming(t, unfiltered, waitForEventID, "wait_for_event")
	assert.Contains(t, detail, "order.created")
	detail = refusedNaming(t, exitAdded, waitForEventID, "wait_for_event")
	assert.Contains(t, detail, "call.booked")
}

// A Wait for event's name is its Step's for the Automation's life (ADR 0136): renaming it, giving two
// of them one name, and giving a new one the name a Wait for event of an earlier version had are
// refused, naming the Step.
func TestExternalAutomationRefusesAWaitForEventNameThatIsNotItsOwn(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, append(slices.Clone(stepAuthorScopes), apitokens.ScopeAutomationsPublish)...)
	ctx := t.Context()

	_, twice := saveWaitForEvent(t, c, nil, purchaseWait(waitForEventID, "purchase"), purchaseWait(otherWaitID, "purchase"))
	a, err := saveWaitForEvent(t, c, nil, purchaseWait(waitForEventID, "purchase"))
	require.NoError(t, err)
	_, renamed := editSteps(t, c, a, purchaseWait(waitForEventID, "order"))
	published, err := c.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: a.Revision}, externalapi.AutomationsPublishParams{ID: a.ID})
	require.NoError(t, err)
	_, reused := editSteps(t, c, published, purchaseWait(otherWaitID, "purchase"))

	detail := refusedNaming(t, twice, otherWaitID, "wait_for_event")
	assert.Contains(t, detail, waitForEventID)
	detail = refusedNaming(t, renamed, waitForEventID, "wait_for_event")
	assert.Contains(t, detail, `"purchase"`)
	detail = refusedNaming(t, reused, otherWaitID, "wait_for_event")
	assert.Contains(t, detail, waitForEventID)
}

// A Wait for event saved again under its own id keeps its name: an edit of its timeout is accepted.
func TestExternalAutomationKeepsAWaitForEventsNameAcrossEdits(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	a, err := saveWaitForEvent(t, c, nil, purchaseWait(waitForEventID, "purchase"))
	require.NoError(t, err)

	_, err = editSteps(t, c, a, `{"id":"`+waitForEventID+`","type":"wait_for_event","name":"purchase","timeoutSeconds":60,`+
		`"awaits":{"kind":"event","matchers":[{"action":"order.created"}]},"arrived":{"steps":[]},"timedOut":{"steps":[]}}`)

	require.NoError(t, err)
}

// What the contract does not admit in a Wait for event is refused with a 400 naming its path: a
// missing timeout, a timeout of zero, a name not of lower-case letters, digits and underscores starting with a letter,
// no awaited matcher, a field it does not hold, and a test of the Enrollment's own message or of an
// HTTP step's response in its Condition, which are a Branch's alone (ADR 0181).
func TestExternalAutomationRefusesAWaitForEventOutsideTheContract(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(step map[string]any)
		path string
	}{
		{"a missing timeout", func(step map[string]any) { delete(step, "timeoutSeconds") }, "steps.steps.0"},
		{"a timeout of zero", func(step map[string]any) { step["timeoutSeconds"] = 0 }, "steps.steps.0.timeoutSeconds"},
		{"a name of the wrong form", func(step map[string]any) { step["name"] = "1st Purchase" }, "steps.steps.0.name"},
		{"no awaited matcher", func(step map[string]any) { step["awaits"].(map[string]any)["matchers"] = []any{} }, "steps.steps.0.awaits.matchers"},
		{"an unknown field", func(step map[string]any) { step["seconds"] = 3 }, "steps.steps.0.seconds"},
		{"a test of the enrollment's own message", func(step map[string]any) {
			step["awaits"].(map[string]any)["condition"] = testhelper.NestedCondition(t,
				`{"kind":"stepMessage","step":"`+thankedID+`","engagement":"opened"}`)
		}, "steps.steps.0.awaits.condition.entries.1.entries.1"},
		{"a test of an http step's response", func(step map[string]any) {
			step["awaits"].(map[string]any)["condition"] = testhelper.NestedCondition(t,
				`{"kind":"property","of":{"kind":"http","name":"coupon"},"key":"code","test":{"kind":"text","operator":"eq","value":"X1"}}`)
		}, "steps.steps.0.awaits.condition.entries.1.entries.1.of"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testhelper.Setup(t)
			edited := env.ExternalVia(t, env.ScopedBearer(t, apitokens.ScopeAutomationsWrite), testhelper.EditBody(func(body map[string]any) {
				tc.edit(body["steps"].(map[string]any)["steps"].([]any)[0].(map[string]any))
			}))

			_, err := saveWaitForEvent(t, edited, nil, purchaseWait(waitForEventID, "purchase"))

			refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
			assert.Equal(t, []string{tc.path}, slices.Collect(maps.Keys(refused.Errors.Or(nil))))
		})
	}
}

// welcomeID is the email Step a Wait for event on an open of its message names.
const welcomeID = "7d2a4b3c-9e5f-4a6b-8c7d-2e3f4a5b6c06"

// openWait is a Wait for event that waits two days for an open of the email welcomeID, its unknown
// path holding the Steps written as unknown ("" for none).
func openWait(unknown string) string {
	doc := `{"id":"` + waitForEventID + `","type":"wait_for_event","name":"welcome_open","timeoutSeconds":172800,` +
		`"awaits":{"kind":"message","step":"` + welcomeID + `","engagement":"opened"},` +
		`"arrived":{"id":"` + arrivedPathID + `","steps":[]},"timedOut":{"id":"` + timedOutPathID + `","steps":[]}`
	if unknown != "" {
		doc += `,"unknown":` + unknown
	}
	return doc + `}`
}

// saveOpenWait creates an Automation whose root path sends the welcome email, then waits for its
// open as the Wait for event written as doc.
func saveOpenWait(t *testing.T, c *externalapi.Client, doc string) (*externalapi.AutomationResource, error) {
	t.Helper()
	welcome := externalapi.EmailStepInput{ID: externalapi.NewOptStepId(welcomeID), Subject: "Welcome", Body: externalapi.NewOptString("<p>Hi</p>")}
	return c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Waits for an open", Trigger: testhelper.ExternalTrigger("signup"),
		Steps: externalapi.NewOptAutomationPathInput(testhelper.ExternalPath(welcome, testhelper.ExternalWaitForEvent(t, doc))),
	})
}

// A Wait for event on an open of a message round-trips through /api as {awaits: {kind: "message",
// step, engagement}} with its unknown path.
func TestExternalAutomationKeepsAWaitForAnOpenWithItsUnknownPath(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)
	written := openWait(`{"id":"` + otherWaitID + `","steps":[{"id":"` + thankedID + `","type":"apply_tag","tag":"untracked"}]}`)
	created, err := saveOpenWait(t, c, written)
	require.NoError(t, err)

	got, err := c.AutomationsGet(t.Context(), externalapi.AutomationsGetParams{ID: created.ID})

	require.NoError(t, err)
	steps := got.Draft.Value.Steps.Steps
	require.Len(t, steps, 2)
	assert.JSONEq(t, written, testhelper.JSONOf(t, &steps[1].OneOf.WaitForEventStep))
}

// Publish refuses a Wait for event on an open without its unknown path (422), naming it; the draft
// keeps it.
func TestExternalPublishRefusesAWaitForAnOpenWithoutItsUnknownPath(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, append(slices.Clone(stepAuthorScopes), apitokens.ScopeAutomationsPublish)...)
	a, err := saveOpenWait(t, c, openWait(""))
	require.NoError(t, err)

	_, err = c.AutomationsPublish(t.Context(), &externalapi.RevisionInput{Revision: a.Revision}, externalapi.AutomationsPublishParams{ID: a.ID})

	detail := refusedNaming(t, err, waitForEventID, "wait_for_event")
	assert.Contains(t, detail, "unknown path")
}

// What a Wait for event awaits is checked against the Workspace at save (422), naming the Step: a
// matcher's filter naming a Tag the Workspace does not have, and a Condition testing a Custom field
// it does not have.
func TestExternalAutomationRefusesAWaitForEventNamingWhatTheWorkspaceLacks(t *testing.T) {
	for name, tc := range map[string]struct{ awaits, named string }{
		"a filter's tag": {`{"kind":"event","matchers":[{"action":"contact.tagged","filter":{"kind":"group","combinator":"and","entries":[` +
			`{"kind":"property","key":"tag","test":{"kind":"text","operator":"eq","value":"no-such-tag"}}]}}]}`, "no-such-tag"},
		"a condition's custom field": {`{"kind":"event","matchers":[{"action":"order.created"}],"condition":{"kind":"group","combinator":"and","entries":[` +
			`{"kind":"customField","key":"no_such_field","test":{"kind":"presence","operator":"set"}}]}}`, "no_such_field"},
	} {
		t.Run(name, func(t *testing.T) {
			env := testhelper.Setup(t)
			c := env.ExternalScoped(t, stepAuthorScopes...)

			_, err := saveWaitForEvent(t, c, nil, `{"id":"`+waitForEventID+`","type":"wait_for_event","name":"purchase","timeoutSeconds":60,`+
				`"awaits":`+tc.awaits+`,"arrived":{"steps":[]},"timedOut":{"steps":[]}}`)

			detail := refusedNaming(t, err, waitForEventID, "wait_for_event")
			assert.Contains(t, detail, tc.named)
		})
	}
}
