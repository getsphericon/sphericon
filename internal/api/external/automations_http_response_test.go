package external_test

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The ids the HTTP steps of these tests are written with.
const (
	couponCallID  = "5c1e2d3f-4a5b-4c6d-8e7f-0a1b2c3d4e01"
	loyaltyCallID = "5c1e2d3f-4a5b-4c6d-8e7f-0a1b2c3d4e02"
)

// call is an HTTP step named name, with the id given, as a client writes it.
func call(id, name string) externalapi.HttpStepInput {
	return externalapi.HttpStepInput{
		ID:   externalapi.NewOptStepId(externalapi.StepId(id)),
		Name: externalapi.StepName(name),
		Body: "{}",
		URL:  externalapi.NewOptString("https://hooks.example.com/coupons"),
	}
}

// saveCalls creates an Automation whose root path holds the Steps given.
func saveCalls(t *testing.T, c *externalapi.Client, steps ...any) (*externalapi.AutomationResource, error) {
	t.Helper()
	return c.AutomationsCreate(t.Context(), &externalapi.CreateAutomationInput{
		Name: "Your coupon", Trigger: testhelper.ExternalTrigger("signup"), Steps: testhelper.ExternalPathEdit(steps...),
	})
}

// editCalls writes the Steps of the Automation's draft.
func editCalls(t *testing.T, c *externalapi.Client, a *externalapi.AutomationResource, steps ...any) (*externalapi.AutomationResource, error) {
	t.Helper()
	return c.AutomationsUpdate(t.Context(), &externalapi.UpdateAutomationInput{Revision: a.Revision, Steps: testhelper.ExternalPathEdit(steps...)},
		externalapi.AutomationsUpdateParams{ID: a.ID})
}

// An HTTP step's name is its Step's for the Automation's life (ADR 0161): renaming it, giving two of
// them one name, giving a Wait for event the name an HTTP step holds, and giving a new one the name
// an HTTP step of an earlier version had are refused, naming the Step: a name is unique in the
// Automation.
func TestExternalAutomationRefusesAnHTTPStepNameThatIsNotItsOwn(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, append(slices.Clone(stepAuthorScopes), apitokens.ScopeAutomationsPublish)...)
	ctx := t.Context()

	_, twice := saveCalls(t, c, call(couponCallID, "coupon"), call(loyaltyCallID, "coupon"))
	_, beside := saveCalls(t, c, call(couponCallID, "coupon"), testhelper.ExternalWaitForEvent(t, purchaseWait(waitForEventID, "coupon")))
	a, err := saveCalls(t, c, call(couponCallID, "coupon"))
	require.NoError(t, err)
	_, renamed := editCalls(t, c, a, call(couponCallID, "voucher"))
	published, err := c.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: a.Revision}, externalapi.AutomationsPublishParams{ID: a.ID})
	require.NoError(t, err)
	_, reused := editCalls(t, c, published, call(loyaltyCallID, "coupon"))

	detail := refusedNaming(t, twice, loyaltyCallID, "http")
	assert.Contains(t, detail, couponCallID)
	detail = refusedNaming(t, beside, waitForEventID, "wait_for_event")
	assert.Contains(t, detail, couponCallID)
	detail = refusedNaming(t, renamed, couponCallID, "http")
	assert.Contains(t, detail, `"coupon"`)
	detail = refusedNaming(t, reused, loyaltyCallID, "http")
	assert.Contains(t, detail, couponCallID)
}

// An HTTP step needs a name of lower-case letters, digits and underscores starting with a letter:
// one of another form is refused with a 400 naming its path.
func TestExternalAutomationRefusesAnHTTPStepNameOfAnotherForm(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	_, err := saveCalls(t, c, call(couponCallID, "Coupon Code"))

	refused := testhelper.ExternalRefusal(t, err, http.StatusBadRequest)
	assert.Contains(t, refused.Detail.Or(""), "name")
}

// responseIs is a Branch path's test that the code the HTTP step name kept is X1.
func responseIs(name string) string {
	return `{"kind":"group","combinator":"and","entries":[{"kind":"property","of":{"kind":"http","name":"` + name + `"},` +
		`"key":"code","test":{"kind":"text","operator":"eq","value":"X1"}}]}`
}

// branchOn is a Branch whose one path holds the Condition written as doc, as a client writes it.
func branchOn(t *testing.T, doc string) externalapi.BranchStepInput {
	t.Helper()
	return testhelper.ExternalBranch(t, `{"type":"branch","paths":[{"condition":`+doc+`,"steps":[]}],"otherwise":{"steps":[]}}`)
}

// A Branch may test the response of an HTTP step that can come before it; one naming an HTTP step
// after it, or none of the version, is refused, naming the Branch.
func TestExternalAutomationBranchTestsOnlyAnEarlierHTTPStepsResponse(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, stepAuthorScopes...)

	_, after := saveCalls(t, c, call(couponCallID, "coupon"), branchOn(t, responseIs("coupon")))
	_, before := saveCalls(t, c, branchOn(t, responseIs("coupon")), call(couponCallID, "coupon"))
	_, nobody := saveCalls(t, c, call(couponCallID, "coupon"), branchOn(t, responseIs("loyalty")))

	require.NoError(t, after)
	detail := refusedNaming(t, before, "", "branch")
	assert.Contains(t, detail, `"coupon"`)
	detail = refusedNaming(t, nobody, "", "branch")
	assert.Contains(t, detail, `"loyalty"`)
}

// hookAnswer is a hook that answers every call with status and body.
type hookAnswer struct {
	status int
	body   string
}

func (h hookAnswer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: h.status, Body: io.NopCloser(strings.NewReader(h.body))}, nil
}

// couponing publishes an Automation on trial.started whose HTTP step `coupon` is followed by an
// hour's wait, enrolls Alice, makes the call with the hook answering body and returns the ids of
// the Automation and of her Enrollment, standing on the wait.
func couponing(t *testing.T, env *testhelper.TestEnv, body string) (automationID, runID int64) {
	t.Helper()
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	steps := automationstep.Root(
		automationstep.New(automationstep.HTTP{Name: "coupon", URL: "https://hooks.example.com/coupons", Body: "{}"}),
		automationstep.New(automationstep.Wait{Seconds: 3600}),
	)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Your coupon", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: "trial.started"}},
		ExitEvents: []eventmatcher.Matcher{{Action: "subscription.cancelled"}}, Steps: &steps,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	recordFor(t, env, "trial.started", nil)
	runID, err = env.DB.AutomationRun.Query().Where(automationrun.AutomationID(a.ID)).OnlyID(ctx)
	require.NoError(t, err)
	unfenced := func(ctx context.Context, _ *ent.Scoped, _ int64, effect func(context.Context) error) error {
		return effect(ctx)
	}
	stepper := automations.NewStepExecution(env.Bus, env.OutboundModule(), testhelper.Module[*contacts.Module](env), unfenced, env.Cipher, hookAnswer{status: http.StatusOK, body: body}, time.Now)
	_, err = stepper.ExecuteHTTP(ctx, s, automations.ExecuteHTTPInput{
		RunID: runID, StepID: env.CurrentStepID(t, runID), Attempt: httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: time.Now()},
	})
	require.NoError(t, err)
	return a.ID, runID
}

// An Enrollment returns the response its HTTP step kept by the Step's name while it runs, in the
// list and alone, and no longer once it ends.
func TestExternalEnrollmentsReturnTheKeptResponseWhileTheEnrollmentRuns(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	c := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)
	automationID, runID := couponing(t, env, `{"code":"X1"}`)
	get := externalapi.EnrollmentsGetParams{AutomationId: entityIDString(automationID), EnrollmentId: entityIDString(runID)}

	running, err := c.EnrollmentsGet(ctx, get)
	require.NoError(t, err)
	listed, err := c.EnrollmentsList(ctx, externalapi.EnrollmentsListParams{AutomationId: entityIDString(automationID)})
	require.NoError(t, err)
	recordFor(t, env, "subscription.cancelled", nil)
	ended, err := c.EnrollmentsGet(ctx, get)
	require.NoError(t, err)

	require.Len(t, running.HTTP, 1)
	kept := running.HTTP[0]
	assert.Equal(t, "coupon", kept.Name)
	assert.Equal(t, int32(http.StatusOK), kept.Status)
	assert.JSONEq(t, `"X1"`, string(kept.Body.Value["code"]))
	assert.False(t, kept.Dropped.Set)
	require.Len(t, listed.Items, 1)
	assert.Equal(t, running.HTTP, listed.Items[0].HTTP, "the list returns it too")
	assert.Equal(t, externalapi.EnrollmentStatusExited, ended.Status)
	assert.Empty(t, ended.HTTP, "cleared when it ended")
}

// A body the step could not keep is returned as dropped, with why, and no body.
func TestExternalEnrollmentsSayWhyAnHTTPStepDroppedTheBody(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, apitokens.ScopeEnrollmentsRead)
	automationID, runID := couponing(t, env, `["X1"]`)

	running, err := c.EnrollmentsGet(t.Context(), externalapi.EnrollmentsGetParams{AutomationId: entityIDString(automationID), EnrollmentId: entityIDString(runID)})

	require.NoError(t, err)
	require.Len(t, running.HTTP, 1)
	assert.False(t, running.HTTP[0].Body.Set)
	assert.Equal(t, externalapi.NewOptHttpBodyDropped(externalapi.HttpBodyDroppedNotObject), running.HTTP[0].Dropped)
}
