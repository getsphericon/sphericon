//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
)

// Automation is what a scenario chooses about an Automation: one email step, no waits.
type Automation struct {
	Name         string
	TriggerEvent string
	Subject      string
	// Body is HTML.
	Body string
}

// PublishAutomation creates an Automation with a single email step (no wait steps, so
// the email goes out as soon as a Contact is enrolled) and publishes it. Creation
// always stores an inactive Automation; publishing is a separate step. It returns the Automation's id.
func (w *Workspace) PublishAutomation(a Automation) int64 {
	w.t.Helper()
	ctx := w.t.Context()
	if a.Name == "" {
		a.Name = "e2e automation " + uniq()
	}
	res, err := w.api.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:    externalapi.Name(a.Name),
		Trigger: eventTrigger(a.TriggerEvent),
		Steps:   rootPath(emailStep(externalapi.EmailStepInput{Subject: a.Subject, Body: externalapi.NewOptString(a.Body)})),
	})
	created := ok[externalapi.AutomationResource](w.t, "create automation", res, err)

	published, err := w.api.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: created.Revision}, externalapi.AutomationsPublishParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "publish automation", published, err)
	require.Equal(w.t, externalapi.AutomationStatusActive, got.Status)
	return int64(got.ID)
}

// eventTrigger is the Trigger on the Event action, with no filter.
func eventTrigger(action string) externalapi.AutomationTriggerInput {
	return externalapi.AutomationTriggerInput{OneOf: externalapi.NewEventTriggerInputAutomationTriggerInputSum(
		externalapi.EventTriggerInput{Kind: externalapi.EventTriggerInputKindEvent, Action: action},
	)}
}

// AuthoredAutomation is what a scenario chooses about an Automation whose one email step is
// written in the editor: its document is one paragraph of Text, and the server builds the body.
type AuthoredAutomation struct {
	TriggerEvent string
	Subject      string
	// Text is the paragraph of the step's document; merge tags are allowed.
	Text string
}

// PublishAuthoredAutomation saves an Automation with a single email step that holds an editor
// document through the external API and publishes it.
func (w *Workspace) PublishAuthoredAutomation(a AuthoredAutomation) {
	w.t.Helper()
	ctx := w.t.Context()
	doc, err := json.Marshal(map[string]any{
		"type": "doc",
		"content": []any{map[string]any{
			"type":    "paragraph",
			"content": []any{map[string]any{"type": "text", "text": a.Text}},
		}},
	})
	require.NoError(w.t, err)

	res, err := w.api.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:    externalapi.Name("e2e authored automation " + uniq()),
		Trigger: eventTrigger(a.TriggerEvent),
		Steps: rootPath(emailStep(externalapi.EmailStepInput{
			Subject: a.Subject,
			Doc:     externalapi.NewOptEditorDocument(docObject[externalapi.EditorDocument](w.t, doc)),
		})),
	})
	created := ok[externalapi.AutomationResource](w.t, "create authored automation", res, err)
	published, err := w.api.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: created.Revision}, externalapi.AutomationsPublishParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "publish authored automation", published, err)
	require.Equal(w.t, externalapi.AutomationStatusActive, got.Status)
}

// HTTPAutomation is what a scenario chooses about an Automation that calls a Hook and then
// sends one email.
type HTTPAutomation struct {
	TriggerEvent string
	Hook         *Hook
	// Body is the Liquid JSON body of the HTTP step.
	Body string
	// Subject and Body of the email step that follows the HTTP step.
	Subject   string
	EmailBody string
}

// PublishHTTPAutomation creates an Automation whose first step POSTs to the Hook (signed
// when the Hook has a secret) and whose second step is an email, and publishes it.
func (w *Workspace) PublishHTTPAutomation(a HTTPAutomation) {
	w.t.Helper()
	ctx := w.t.Context()
	httpStep := externalapi.HttpStepInput{
		URL:  externalapi.NewOptString(a.Hook.URL),
		Body: a.Body,
	}
	if a.Hook.Secret != "" {
		httpStep.Secret = externalapi.NewOptSigningSecret(externalapi.SigningSecret(a.Hook.Secret))
	}
	res, err := w.api.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:    externalapi.Name("e2e http automation " + uniq()),
		Trigger: eventTrigger(a.TriggerEvent),
		Steps: rootPath(
			externalapi.AutomationStepInput{OneOf: externalapi.NewHttpStepInputAutomationStepInputSum(httpStep)},
			emailStep(externalapi.EmailStepInput{Subject: a.Subject, Body: externalapi.NewOptString(a.EmailBody)}),
		),
	})
	created := ok[externalapi.AutomationResource](w.t, "create automation", res, err)

	published, err := w.api.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: created.Revision}, externalapi.AutomationsPublishParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "publish automation", published, err)
	require.Equal(w.t, externalapi.AutomationStatusActive, got.Status)
}

// KeyedAutomation is what a scenario chooses about an Automation keyed by an Event property (ADR
// 0165): one email step, then a month's wait that keeps each Enrollment active, ended early by the
// exit event.
type KeyedAutomation struct {
	TriggerEvent string
	// Key is the property of the trigger event each Enrollment runs for, one per value.
	Key       string
	ExitEvent string
	// Subject may name the Enrollment's event (`{{ event.properties.order_id }}`).
	Subject string
}

// PublishKeyedAutomation creates the keyed Automation and publishes it. It returns its id.
func (w *Workspace) PublishKeyedAutomation(a KeyedAutomation) externalapi.EntityId {
	w.t.Helper()
	ctx := w.t.Context()
	res, err := w.api.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name: externalapi.Name("e2e keyed automation " + uniq()),
		Trigger: externalapi.AutomationTriggerInput{OneOf: externalapi.NewEventTriggerInputAutomationTriggerInputSum(externalapi.EventTriggerInput{
			Kind: externalapi.EventTriggerInputKindEvent, Action: a.TriggerEvent, EnrollmentKey: externalapi.NewOptString(a.Key),
		})},
		ExitEvents: []externalapi.EventMatcherInput{{Action: a.ExitEvent}},
		Steps: rootPath(
			emailStep(externalapi.EmailStepInput{Subject: a.Subject, Body: externalapi.NewOptString(HTML("Thank you for your order"))}),
			externalapi.AutomationStepInput{OneOf: externalapi.NewWaitStepInputAutomationStepInputSum(externalapi.WaitStepInput{
				Type: externalapi.WaitStepInputTypeWait, Seconds: 30 * 24 * 3600,
			})},
		),
	})
	created := ok[externalapi.AutomationResource](w.t, "create keyed automation", res, err)
	published, err := w.api.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: created.Revision}, externalapi.AutomationsPublishParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "publish keyed automation", published, err)
	require.Equal(w.t, externalapi.AutomationStatusActive, got.Status)
	return got.ID
}

// EnrollmentsByKey is the status of each Enrollment of the Automation, by its key value, read
// through the public API.
func (w *Workspace) EnrollmentsByKey(automationID externalapi.EntityId) map[string]externalapi.EnrollmentStatus {
	w.t.Helper()
	listed, err := w.api.EnrollmentsList(w.t.Context(), externalapi.EnrollmentsListParams{AutomationId: automationID})
	require.NoError(w.t, err, "list enrollments")
	byKey := map[string]externalapi.EnrollmentStatus{}
	for i := range listed.Items {
		byKey[listed.Items[i].KeyValue.Or("")] = listed.Items[i].Status
	}
	return byKey
}

// rootPath is a version's Steps as a write sends them: the root path, its id and theirs left to
// the server.
func rootPath(steps ...externalapi.AutomationStepInput) externalapi.OptAutomationPathInput {
	return externalapi.NewOptAutomationPathInput(externalapi.AutomationPathInput{Steps: steps})
}

// emailStep is an email step as a member of the Step union.
func emailStep(st externalapi.EmailStepInput) externalapi.AutomationStepInput {
	return externalapi.AutomationStepInput{OneOf: externalapi.NewEmailStepInputAutomationStepInputSum(st)}
}

// RecordEvent records an event for the Contact with the given email through the
// public API. Ingest is accept-then-process: observe the effect with Inbox.Wait.
func (w *Workspace) RecordEvent(email, action string) {
	w.t.Helper()
	err := w.api.EventsCreate(w.t.Context(), &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{
			SubjectId: "subject-" + email,
			Action:    action,
			Email:     externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(email)),
		}},
	})
	require.NoError(w.t, err, "record event")
}

// RecordEventWith records an event with the properties for the Contact with the given email
// through the public API, each property a string. Ingest is accept-then-process: observe the effect
// with Inbox.Wait.
func (w *Workspace) RecordEventWith(email, action string, properties map[string]string) {
	w.t.Helper()
	props := externalapi.JsonObject{}
	for key, value := range properties {
		raw, err := json.Marshal(value)
		require.NoError(w.t, err)
		props[key] = raw
	}
	err := w.api.EventsCreate(w.t.Context(), &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{
			SubjectId:  "subject-" + email,
			Action:     action,
			Email:      externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(email)),
			Properties: externalapi.NewOptNilJsonObject(props),
		}},
	})
	require.NoError(w.t, err, "record event")
}

// OpenMail opens a tracked email the way a mail client does: it loads the open pixel from the
// email's HTML, and requires success. The open is recorded asynchronously, so a scenario observes
// its effect (an Automation's email) with Inbox.Wait.
func (w *Workspace) OpenMail(msg Message) {
	w.t.Helper()
	pixel := msg.Image("/e/o/")
	require.NotEmpty(w.t, pixel, "the email %q carries an open pixel", msg.Subject)
	req, err := http.NewRequestWithContext(w.t.Context(), http.MethodGet, pixel, http.NoBody)
	require.NoError(w.t, err)
	// A person's mail client, not Go's default agent: that one is machine engagement (ADR 0053).
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	res, err := http.DefaultClient.Do(req)
	require.NoError(w.t, err)
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(w.t, err)
	require.NoError(w.t, res.Body.Close())
	require.Equal(w.t, http.StatusOK, res.StatusCode, "open pixel %s", pixel)
}

// BranchAutomation is what a scenario chooses about an Automation that sends each Enrollment down a
// path by a property of the Event that began it (ADR 0124): a Branch whose one path holds when the
// property Key equals Value and sends PathSubject, whose otherwise sends OtherwiseSubject, and an
// email after the Branch, AfterSubject, that both paths' Contacts receive.
type BranchAutomation struct {
	TriggerEvent     string
	Key, Value       string
	PathSubject      string
	OtherwiseSubject string
	AfterSubject     string
}

// PublishBranchAutomation creates the Automation with its Branch through the public API and
// publishes it.
func (w *Workspace) PublishBranchAutomation(a BranchAutomation) {
	w.t.Helper()
	ctx := w.t.Context()
	leaf, err := json.Marshal(map[string]any{
		"kind": "group", "combinator": "and", "entries": []any{map[string]any{
			"kind": "property", "of": map[string]any{"kind": "trigger"}, "key": a.Key,
			"test": map[string]any{"kind": "text", "operator": "eq", "value": a.Value},
		}},
	})
	require.NoError(w.t, err)
	var cond externalapi.BranchConditionInput
	require.NoError(w.t, cond.UnmarshalJSON(leaf))
	email := func(subject string) externalapi.EmailStepInput {
		return externalapi.EmailStepInput{Subject: subject, Body: externalapi.NewOptString(HTML(subject))}
	}
	branch := externalapi.BranchStepInput{
		Type: externalapi.BranchStepInputTypeBranch,
		Paths: []externalapi.BranchPathInput{{
			Condition: cond,
			Steps:     []externalapi.PathStepInput{{OneOf: externalapi.NewEmailStepInputPathStepInputSum(email(a.PathSubject))}},
		}},
		Otherwise: externalapi.AutomationPathInput{Steps: []externalapi.AutomationStepInput{emailStep(email(a.OtherwiseSubject))}},
	}
	res, err := w.api.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:    externalapi.Name("e2e branch automation " + uniq()),
		Trigger: eventTrigger(a.TriggerEvent),
		Steps: rootPath(
			externalapi.AutomationStepInput{OneOf: externalapi.NewBranchStepInputAutomationStepInputSum(branch)},
			emailStep(email(a.AfterSubject)),
		),
	})
	created := ok[externalapi.AutomationResource](w.t, "create branch automation", res, err)
	published, err := w.api.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: created.Revision}, externalapi.AutomationsPublishParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "publish branch automation", published, err)
	require.Equal(w.t, externalapi.AutomationStatusActive, got.Status)
}

// WaitForEventAutomation is what a scenario chooses about an Automation that waits for an Event
// after it enrolls a Contact (ADR 0124): a Wait for event named Name on Awaited for Timeout, whose
// arrived path sends ArrivedSubject with the text ArrivedBody, which may print the awaited Event as
// `{{ awaited.<Name>.properties.<key> }}` (ADR 0136), and whose timed out path sends
// TimedOutSubject, then an email after it, AfterSubject, that both paths' Contacts receive.
type WaitForEventAutomation struct {
	TriggerEvent    string
	Name            string
	Awaited         string
	Timeout         time.Duration
	ArrivedSubject  string
	ArrivedBody     string
	TimedOutSubject string
	AfterSubject    string
}

// PublishWaitForEventAutomation creates the Automation with its Wait for event through the public
// API and publishes it. It returns its id.
func (w *Workspace) PublishWaitForEventAutomation(a WaitForEventAutomation) externalapi.EntityId {
	w.t.Helper()
	ctx := w.t.Context()
	emailWith := func(subject, body string) externalapi.AutomationStepInput {
		return emailStep(externalapi.EmailStepInput{Subject: subject, Body: externalapi.NewOptString(HTML(body))})
	}
	email := func(subject string) externalapi.AutomationStepInput { return emailWith(subject, subject) }
	wait := externalapi.WaitForEventStepInput{
		Type:           externalapi.WaitForEventStepInputTypeWaitForEvent,
		Name:           externalapi.WaitForEventName(a.Name),
		TimeoutSeconds: narrow.Int32(int64(a.Timeout / time.Second)),
		Awaits: externalapi.WaitForEventAwaitsInput{OneOf: externalapi.NewAwaitedEventInputWaitForEventAwaitsInputSum(
			externalapi.AwaitedEventInput{Kind: externalapi.AwaitedEventInputKindEvent, Matchers: []externalapi.EventMatcherInput{{Action: a.Awaited}}},
		)},
		Arrived:  externalapi.AutomationPathInput{Steps: []externalapi.AutomationStepInput{emailWith(a.ArrivedSubject, a.ArrivedBody)}},
		TimedOut: externalapi.AutomationPathInput{Steps: []externalapi.AutomationStepInput{email(a.TimedOutSubject)}},
	}
	res, err := w.api.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:    externalapi.Name("e2e wait for event automation " + uniq()),
		Trigger: eventTrigger(a.TriggerEvent),
		Steps: rootPath(
			externalapi.AutomationStepInput{OneOf: externalapi.NewWaitForEventStepInputAutomationStepInputSum(wait)},
			email(a.AfterSubject),
		),
	})
	created := ok[externalapi.AutomationResource](w.t, "create wait for event automation", res, err)
	published, err := w.api.AutomationsPublish(ctx, &externalapi.RevisionInput{Revision: created.Revision}, externalapi.AutomationsPublishParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "publish wait for event automation", published, err)
	require.Equal(w.t, externalapi.AutomationStatusActive, got.Status)
	return got.ID
}

// WaitingEnrollments is how many active Enrollments of the Automation stand on a Step that holds
// them until a moment (a wait, or a Wait for event until its timeout), read through the public API.
func (w *Workspace) WaitingEnrollments(automationID externalapi.EntityId) int {
	w.t.Helper()
	listed, err := w.api.EnrollmentsList(w.t.Context(), externalapi.EnrollmentsListParams{
		AutomationId: automationID, Status: externalapi.NewOptEnrollmentStatus(externalapi.EnrollmentStatusActive),
	})
	require.NoError(w.t, err, "list enrollments")
	waiting := 0
	for i := range listed.Items {
		if listed.Items[i].ResumeAt.Set {
			waiting++
		}
	}
	return waiting
}
