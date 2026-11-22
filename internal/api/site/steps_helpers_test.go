package site_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
)

// storedEmail is the email step at i of a stored path.
func storedEmail(t *testing.T, p automationstep.Path, i int) automationstep.Email {
	t.Helper()
	require.Greater(t, len(p.Steps), i)
	email, ok := p.Steps[i].Payload.(automationstep.Email)
	require.True(t, ok, "step %d is an email step", i)
	return email
}

// storedHTTP is the HTTP step at i of a stored path.
func storedHTTP(t *testing.T, p automationstep.Path, i int) automationstep.HTTP {
	t.Helper()
	require.Greater(t, len(p.Steps), i)
	call, ok := p.Steps[i].Payload.(automationstep.HTTP)
	require.True(t, ok, "step %d is an HTTP step", i)
	return call
}

// siteStepIDs are the ids of a version's Steps, in order.
func siteStepIDs(p siteapi.AutomationPath) []string {
	ids := make([]string, len(p.Steps))
	for i := range p.Steps {
		switch s := &p.Steps[i].OneOf; s.Type {
		case siteapi.EmailStepAutomationStepSum:
			ids[i] = string(s.EmailStep.ID)
		case siteapi.TelegramStepAutomationStepSum:
			ids[i] = string(s.TelegramStep.ID)
		case siteapi.SmsStepAutomationStepSum:
			ids[i] = string(s.SmsStep.ID)
		case siteapi.WaitStepAutomationStepSum:
			ids[i] = string(s.WaitStep.ID)
		case siteapi.ApplyTagStepAutomationStepSum:
			ids[i] = string(s.ApplyTagStep.ID)
		case siteapi.RemoveTagStepAutomationStepSum:
			ids[i] = string(s.RemoveTagStep.ID)
		case siteapi.HttpStepAutomationStepSum:
			ids[i] = string(s.HttpStep.ID)
		case siteapi.EndStepAutomationStepSum:
			ids[i] = string(s.EndStep.ID)
		}
	}
	return ids
}
