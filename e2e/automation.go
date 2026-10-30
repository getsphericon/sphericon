//go:build e2e

package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
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
		Name:         a.Name,
		TriggerEvent: a.TriggerEvent,
		Steps: []externalapi.AutomationStep{{
			Type:    externalapi.AutomationStepTypeEmail,
			Subject: externalapi.NewOptString(a.Subject),
			Body:    externalapi.NewOptString(a.Body),
		}},
	})
	created := ok[externalapi.AutomationResource](w.t, "create automation", res, err)

	published, err := w.api.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "publish automation", published, err)
	require.Equal(w.t, externalapi.AutomationStatusActive, got.Status)
	return int64(got.ID)
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
		Name:         "e2e authored automation " + uniq(),
		TriggerEvent: a.TriggerEvent,
		Steps: []externalapi.AutomationStep{{
			Type:    externalapi.AutomationStepTypeEmail,
			Subject: externalapi.NewOptString(a.Subject),
			Doc:     externalapi.NewOptAutomationStepDoc(docObject[externalapi.AutomationStepDoc](w.t, doc)),
		}},
	})
	created := ok[externalapi.AutomationResource](w.t, "create authored automation", res, err)
	published, err := w.api.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: created.ID})
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
	httpStep := externalapi.AutomationStep{
		Type: externalapi.AutomationStepTypeHTTP,
		URL:  externalapi.NewOptString(a.Hook.URL),
		Body: externalapi.NewOptString(a.Body),
	}
	if a.Hook.Secret != "" {
		httpStep.Secret = externalapi.NewOptString(a.Hook.Secret)
	}
	res, err := w.api.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:         "e2e http automation " + uniq(),
		TriggerEvent: a.TriggerEvent,
		Steps: []externalapi.AutomationStep{httpStep, {
			Type:    externalapi.AutomationStepTypeEmail,
			Subject: externalapi.NewOptString(a.Subject),
			Body:    externalapi.NewOptString(a.EmailBody),
		}},
	})
	created := ok[externalapi.AutomationResource](w.t, "create automation", res, err)

	published, err := w.api.AutomationsPublish(ctx, externalapi.AutomationsPublishParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "publish automation", published, err)
	require.Equal(w.t, externalapi.AutomationStatusActive, got.Status)
}

// RecordEvent records an event for the Contact with the given email through the
// public API. Ingest is accept-then-process: observe the effect with Inbox.Wait.
func (w *Workspace) RecordEvent(email, action string) {
	w.t.Helper()
	res, err := w.api.EventsCreate(w.t.Context(), &externalapi.RecordEventsInput{
		Events: []externalapi.EventInput{{
			SubjectId: "subject-" + email,
			Action:    action,
			Email:     externalapi.NewOptNilEmailAddress(externalapi.EmailAddress(email)),
		}},
	})
	require.NoError(w.t, err, "record event")
	_, isNoContent := res.(*externalapi.EventsCreateNoContent)
	require.True(w.t, isNoContent, "record event: unexpected response %T: %+v", res, res)
}

// openPixel finds the open pixel a tracked email carries.
var openPixel = regexp.MustCompile(`src="(https?://[^"]*/e/o/[^"]+)"`)

// OpenMail opens a tracked email the way a mail client does: it loads the open pixel from the
// email's HTML, and requires success. The open is recorded asynchronously, so a scenario observes
// its effect (an Automation's email) with Inbox.Wait.
func (w *Workspace) OpenMail(msg Message) {
	w.t.Helper()
	m := openPixel.FindStringSubmatch(msg.HTML)
	require.NotNil(w.t, m, "the email %q carries an open pixel", msg.Subject)
	req, err := http.NewRequestWithContext(w.t.Context(), http.MethodGet, m[1], http.NoBody)
	require.NoError(w.t, err)
	// A person's mail client, not Go's default agent: that one is machine engagement (ADR 0053).
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15")
	res, err := http.DefaultClient.Do(req)
	require.NoError(w.t, err)
	_, err = io.Copy(io.Discard, res.Body)
	require.NoError(w.t, err)
	require.NoError(w.t, res.Body.Close())
	require.Equal(w.t, http.StatusOK, res.StatusCode, "open pixel %s", m[1])
}
