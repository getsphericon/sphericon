//go:build e2e

package e2e

import (
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
)

// Automation is what a scenario chooses about an Automation: one email step, no waits.
type Automation struct {
	Name         string
	TriggerEvent string
	Subject      string
	// Body is MJML.
	Body string
}

// ActivateAutomation creates an Automation with a single email step (no wait steps, so
// the email goes out as soon as a Contact is enrolled) and activates it. Creation
// always stores a draft; activating is a separate step.
func (w *Workspace) ActivateAutomation(a Automation) {
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

	act, err := w.api.AutomationsActivate(ctx, externalapi.AutomationsActivateParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "activate automation", act, err)
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

// ActivateHTTPAutomation creates an Automation whose first step POSTs to the Hook (signed
// when the Hook has a secret) and whose second step is an email, and activates it.
func (w *Workspace) ActivateHTTPAutomation(a HTTPAutomation) {
	w.t.Helper()
	ctx := w.t.Context()
	http := externalapi.AutomationStep{
		Type: externalapi.AutomationStepTypeHTTP,
		URL:  externalapi.NewOptString(a.Hook.URL),
		Body: externalapi.NewOptString(a.Body),
	}
	if a.Hook.Secret != "" {
		http.Secret = externalapi.NewOptString(a.Hook.Secret)
	}
	res, err := w.api.AutomationsCreate(ctx, &externalapi.CreateAutomationInput{
		Name:         "e2e http automation " + uniq(),
		TriggerEvent: a.TriggerEvent,
		Steps: []externalapi.AutomationStep{http, {
			Type:    externalapi.AutomationStepTypeEmail,
			Subject: externalapi.NewOptString(a.Subject),
			Body:    externalapi.NewOptString(a.EmailBody),
		}},
	})
	created := ok[externalapi.AutomationResource](w.t, "create automation", res, err)

	act, err := w.api.AutomationsActivate(ctx, externalapi.AutomationsActivateParams{ID: created.ID})
	got := ok[externalapi.AutomationResource](w.t, "activate automation", act, err)
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
