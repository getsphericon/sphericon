package automations_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationrun"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/eventmatcher"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The kept response (ADR 0161): an HTTP step that gets a 2xx keeps its status and, when it is a JSON
// object, its body, which the Steps after it read as `http.<name>`.

// couponStep is the HTTP step named `coupon` the Automations of these tests start with.
var couponStep = automationstep.HTTP{Name: "coupon", URL: "https://hooks.example.com/coupons", Body: "{}"}

// respondedFlow is a live Automation, on an env whose clock the test moves, whose first Step is
// couponStep, followed by the Steps then.
type respondedFlow struct {
	env          *testhelper.TestEnv
	clock        *testhelper.Clock
	automationID int64
	call         automationstep.ID
}

// respondThen publishes the Automation of respondedFlow on an env of its own.
func respondThen(t *testing.T, then ...automationstep.Step) respondedFlow {
	t.Helper()
	env, clock := awaitedEnv(t)
	ctx := t.Context()
	s := env.DB.Scoped(fixtures.AcmeID)
	m := testhelper.Module[*automations.Module](env)
	root := automationstep.Root(append([]automationstep.Step{{Payload: couponStep}}, then...)...)
	a, err := m.Create(ctx, s, automations.CreateInput{
		Name: "Your coupon", Trigger: automations.EventTrigger{Matcher: eventmatcher.Matcher{Action: awaitedTrigger}}, Steps: &root,
	})
	require.NoError(t, err)
	_, err = m.Publish(ctx, s, a.ID, testhelper.AutomationRevisionIn(ctx, t, s, a.ID))
	require.NoError(t, err)
	live, err := m.Get(ctx, s, a.ID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	return respondedFlow{env: env, clock: clock, automationID: a.ID, call: version.Steps.Steps[0].ID}
}

// enroll enrolls the Contact on the Trigger's Event, occurred now, and returns its Enrollment,
// standing on the HTTP step, its call queued.
func (f respondedFlow) enroll(t *testing.T, c *ent.Contact) int64 {
	t.Helper()
	handStored(t, f.env, c.ID, awaitedTrigger, collected(c.ID, awaitedTrigger, f.clock.Now(), nil))
	runs := runsOf(t, f.env, f.automationID, c.ID)
	require.Len(t, runs, 1)
	return runs[0].ID
}

// answer makes the call of the HTTP step the Enrollment stands on, the hook answering status with
// body.
func (f respondedFlow) answer(t *testing.T, runID int64, status int, body string) {
	t.Helper()
	stepper := automations.NewStepExecution(f.env.Bus, f.env.OutboundModule(), testhelper.Module[*contacts.Module](f.env), noFence, f.env.Cipher, httpResponse{status: status, body: body}, f.clock.Now)
	_, err := stepper.ExecuteHTTP(t.Context(), f.env.DB.Scoped(fixtures.AcmeID), automations.ExecuteHTTPInput{
		RunID: runID, StepID: f.env.CurrentStepID(t, runID), Attempt: httpcall.Attempt{Number: 1, Limit: 3, CreatedAt: f.clock.Now()},
	})
	require.NoError(t, err)
}

// run drives the queued Steps on the env's clock until none is left, unfenced: the env's one
// connection holds the test's transaction. A wait still ahead holds the Enrollment on it.
func (f respondedFlow) run(t *testing.T) {
	t.Helper()
	f.env.RunQueuedSteps(t, automations.NewStepExecution(f.env.Bus, f.env.OutboundModule(), testhelper.Module[*contacts.Module](f.env), noFence, f.env.Cipher, nil, f.clock.Now))
}

// record is the Enrollment's record of the Step step.
func (f respondedFlow) record(t *testing.T, runID int64, step automationstep.ID) *ent.AutomationStepRecord {
	t.Helper()
	records, err := testhelper.Module[*automations.Module](f.env).StepRecords(t.Context(), f.env.DB.Scoped(fixtures.AcmeID), f.automationID, runID)
	require.NoError(t, err)
	found := lo.Filter(records, func(r *ent.AutomationStepRecord, _ int) bool { return r.StepID == step })
	require.Len(t, found, 1)
	return found[0]
}

// couponMail is an email that prints the kept response's status and the body's code.
func couponMail() automationstep.Step {
	return automationstep.Step{Payload: automationstep.Email{Subject: "Your coupon", Body: "<p>Coupon [{{ http.coupon.body.code }}] answered [{{ http.coupon.status }}]</p>"}}
}

// An email after the HTTP step prints the code the hook answered with. A body the step cannot keep
// prints nothing, the status still prints, and the step's record says why the body was dropped.
func TestAnEmailAfterTheHTTPStepPrintsItsResponse(t *testing.T) {
	for name, c := range map[string]struct {
		status  int
		body    string
		prints  string
		dropped *automationstep.BodyDropped
	}{
		"a JSON object": {http.StatusOK, `{"code":"X1"}`, "Coupon [X1] answered [200]", nil},
		"an HTML page":  {http.StatusOK, `<html><body>Thanks</body></html>`, "Coupon [] answered [200]", lo.ToPtr(automationstep.BodyNotJSON)},
		"a JSON array":  {http.StatusCreated, `[{"code":"X1"}]`, "Coupon [] answered [201]", lo.ToPtr(automationstep.BodyNotObject)},
		"over 32 KiB":   {http.StatusOK, `{"code":"X1","pad":"` + strings.Repeat("a", 33<<10) + `"}`, "Coupon [] answered [200]", lo.ToPtr(automationstep.BodyTooLarge)},
		"a NUL inside":  {http.StatusOK, `{"code":"X\u00001"}`, "Coupon [] answered [200]", lo.ToPtr(automationstep.BodyNotJSON)},
	} {
		t.Run(name, func(t *testing.T) {
			f := respondThen(t, automationstep.Step{Payload: automationstep.Wait{Seconds: 60}}, couponMail())
			contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
			runID := f.enroll(t, contact)

			f.answer(t, runID, c.status, c.body)
			record := f.record(t, runID, f.call)
			f.clock.Advance(time.Minute)
			f.run(t)

			sent := f.env.CustomerMail.To(lo.FromPtr(contact.Email))
			require.Len(t, sent, 1)
			assert.Contains(t, sent[0].Text, c.prints)
			status := statusOf(t, f.env, runID)
			assert.Equal(t, c.dropped, record.HTTPDropped)
			assert.Equal(t, automationrun.StatusCompleted, status, "a dropped body moves the Enrollment on all the same")
		})
	}
}

// A Contact's custom field named `http` never stands in for the responses an Enrollment keeps.
func TestACustomFieldNamedHTTPNeverStandsInForTheKeptResponse(t *testing.T) {
	f := respondThen(t, couponMail())
	contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
	_, err := f.env.DB.Scoped(fixtures.AcmeID).Contact().UpdateOneID(contact.ID).
		SetCustomFields(map[string]any{"http": map[string]any{"coupon": map[string]any{"status": 999, "body": map[string]any{"code": "forged"}}}}).
		Save(t.Context())
	require.NoError(t, err)
	runID := f.enroll(t, contact)

	f.answer(t, runID, http.StatusOK, `<p>no json</p>`)
	f.run(t)

	sent := f.env.CustomerMail.To(lo.FromPtr(contact.Email))
	require.Len(t, sent, 1)
	assert.Contains(t, sent[0].Text, "Coupon [] answered [200]")
}

// couponIs is a test that the code the HTTP step coupon kept is X1.
const couponIs = `{"kind":"property","of":{"kind":"http","name":"coupon"},"key":"code","test":{"kind":"text","operator":"eq","value":"X1"}}`

// A Branch testing the kept response's code takes its path when the hook answered X1, and goes
// otherwise when it answered another code or a body the step could not keep: with no copy the test
// fails.
func TestABranchTestsTheKeptResponse(t *testing.T) {
	for name, c := range map[string]struct {
		body string
		path func(b automationstep.Branch) automationstep.ID
	}{
		"the code it tests": {`{"code":"X1"}`, func(b automationstep.Branch) automationstep.ID { return b.Paths[0].ID }},
		"another code":      {`{"code":"Y2"}`, func(b automationstep.Branch) automationstep.ID { return b.Otherwise.ID }},
		"no body kept":      {`not json`, func(b automationstep.Branch) automationstep.ID { return b.Otherwise.ID }},
	} {
		t.Run(name, func(t *testing.T) {
			f := respondThen(t, judging(t, couponIs))
			contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
			runID := f.enroll(t, contact)
			branch := f.branch(t)

			f.answer(t, runID, http.StatusOK, c.body)
			f.run(t)

			taken := f.record(t, runID, judgeID).PathID
			want := c.path(branch)
			assert.Equal(t, want, lo.FromPtr(taken))
		})
	}
}

// branch is the live version's Branch judgeID.
func (f respondedFlow) branch(t *testing.T) automationstep.Branch {
	t.Helper()
	live, err := testhelper.Module[*automations.Module](f.env).Get(t.Context(), f.env.DB.Scoped(fixtures.AcmeID), f.automationID)
	require.NoError(t, err)
	version, _ := automations.LiveAndDraft(live)
	st, ok := automationstep.NewIndex(version.Steps).Step(judgeID)
	require.True(t, ok)
	return st.Payload.(automationstep.Branch)
}

// The kept response is on the HTTP step's record while the Enrollment runs, and the status and the
// body are cleared in the move that ends it; why a body was dropped stays.
func TestTheKeptResponseIsClearedWhenTheEnrollmentEnds(t *testing.T) {
	for name, c := range map[string]struct {
		body    string
		dropped *automationstep.BodyDropped
	}{
		"a kept body":    {`{"code":"X1"}`, nil},
		"a dropped body": {`<html></html>`, lo.ToPtr(automationstep.BodyNotJSON)},
	} {
		t.Run(name, func(t *testing.T) {
			f := respondThen(t, automationstep.Step{Payload: automationstep.Wait{Seconds: 60}})
			contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
			runID := f.enroll(t, contact)
			f.answer(t, runID, http.StatusOK, c.body)
			whileRunning := f.record(t, runID, f.call)

			f.clock.Advance(time.Minute)
			f.run(t)

			afterTheEnd := f.record(t, runID, f.call)
			status := statusOf(t, f.env, runID)
			assert.Equal(t, automationrun.StatusCompleted, status)
			assert.Equal(t, int32(http.StatusOK), lo.FromPtr(whileRunning.HTTPStatus), "kept while it runs")
			assert.Nil(t, afterTheEnd.HTTPStatus, "the status is cleared when it ends")
			assert.Nil(t, afterTheEnd.HTTPBody, "the body is cleared when it ends")
			assert.Equal(t, c.dropped, afterTheEnd.HTTPDropped, "why the body was dropped stays")
		})
	}
}

// An HTTP step that is the Enrollment's last Step completes it and keeps no response, only why it
// dropped a body: nothing comes after it to read one.
func TestAnHTTPStepThatCompletesTheEnrollmentKeepsNoResponse(t *testing.T) {
	f := respondThen(t)
	contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
	runID := f.enroll(t, contact)

	f.answer(t, runID, http.StatusOK, `{"code":"X1"}`)

	record := f.record(t, runID, f.call)
	status := statusOf(t, f.env, runID)
	assert.Equal(t, automationrun.StatusCompleted, status)
	assert.Nil(t, record.HTTPStatus)
	assert.Nil(t, record.HTTPBody)
}

// The Enrollment resource returns the responses its HTTP steps kept, by name, while it runs.
func TestAnEnrollmentReturnsItsKeptResponses(t *testing.T) {
	f := respondThen(t, automationstep.Step{Payload: automationstep.Wait{Seconds: 60}})
	contact := enrollable(t, f.env.DB.Scoped(fixtures.AcmeID), 0)
	runID := f.enroll(t, contact)
	f.answer(t, runID, http.StatusAccepted, `{"code":"X1"}`)

	run, err := testhelper.Module[*automations.Module](f.env).Enrollment(t.Context(), f.env.DB.Scoped(fixtures.AcmeID), f.automationID, runID)
	require.NoError(t, err)
	kept := automations.KeptResponses(run)

	assert.Equal(t, []automations.KeptResponse{{Name: "coupon", Status: http.StatusAccepted, Body: map[string]any{"code": "X1"}}}, kept)
}
