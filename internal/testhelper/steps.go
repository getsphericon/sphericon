package testhelper

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SitePath is a root path of Steps for a site API write, with no id: the server gives it one. Each
// Step is a member of the Step union (siteapi.WaitStepInput, siteapi.EmailStepInput, …) or a
// ready siteapi.AutomationStepInput, so a test writes only the fields it is about.
func SitePath(steps ...any) siteapi.AutomationPathInput {
	out := make([]siteapi.AutomationStepInput, len(steps))
	for i, st := range steps {
		out[i] = SiteStep(st)
	}
	return siteapi.AutomationPathInput{Steps: out}
}

// SitePathEdit is SitePath as the steps of a site API write that may leave them out.
func SitePathEdit(steps ...any) siteapi.OptAutomationPathInput {
	return siteapi.NewOptAutomationPathInput(SitePath(steps...))
}

// SiteStep is one member of the site API's Step union as the union.
func SiteStep(st any) siteapi.AutomationStepInput {
	switch v := st.(type) {
	case siteapi.AutomationStepInput:
		return v
	case siteapi.EmailStepInput:
		return siteapi.AutomationStepInput{OneOf: siteapi.NewEmailStepInputAutomationStepInputSum(v)}
	case siteapi.TelegramStepInput:
		return siteapi.AutomationStepInput{OneOf: siteapi.NewTelegramStepInputAutomationStepInputSum(v)}
	case siteapi.SmsStepInput:
		return siteapi.AutomationStepInput{OneOf: siteapi.NewSmsStepInputAutomationStepInputSum(v)}
	case siteapi.WaitStepInput:
		return siteapi.AutomationStepInput{OneOf: siteapi.NewWaitStepInputAutomationStepInputSum(v)}
	case siteapi.ApplyTagStepInput:
		return siteapi.AutomationStepInput{OneOf: siteapi.NewApplyTagStepInputAutomationStepInputSum(v)}
	case siteapi.RemoveTagStepInput:
		return siteapi.AutomationStepInput{OneOf: siteapi.NewRemoveTagStepInputAutomationStepInputSum(v)}
	case siteapi.HttpStepInput:
		return siteapi.AutomationStepInput{OneOf: siteapi.NewHttpStepInputAutomationStepInputSum(v)}
	case siteapi.EndStepInput:
		return siteapi.AutomationStepInput{OneOf: siteapi.NewEndStepInputAutomationStepInputSum(v)}
	default:
		panic(fmt.Sprintf("testhelper: %T is not a site Step", st))
	}
}

// ExternalPath is SitePath for the external API.
func ExternalPath(steps ...any) externalapi.AutomationPathInput {
	out := make([]externalapi.AutomationStepInput, len(steps))
	for i, st := range steps {
		out[i] = ExternalStep(st)
	}
	return externalapi.AutomationPathInput{Steps: out}
}

// ExternalPathEdit is ExternalPath as the steps of an external API write that may leave them out.
func ExternalPathEdit(steps ...any) externalapi.OptAutomationPathInput {
	return externalapi.NewOptAutomationPathInput(ExternalPath(steps...))
}

// ExternalStep is SiteStep for the external API.
func ExternalStep(st any) externalapi.AutomationStepInput {
	switch v := st.(type) {
	case externalapi.AutomationStepInput:
		return v
	case externalapi.EmailStepInput:
		return externalapi.AutomationStepInput{OneOf: externalapi.NewEmailStepInputAutomationStepInputSum(v)}
	case externalapi.TelegramStepInput:
		return externalapi.AutomationStepInput{OneOf: externalapi.NewTelegramStepInputAutomationStepInputSum(v)}
	case externalapi.SmsStepInput:
		return externalapi.AutomationStepInput{OneOf: externalapi.NewSmsStepInputAutomationStepInputSum(v)}
	case externalapi.WaitStepInput:
		return externalapi.AutomationStepInput{OneOf: externalapi.NewWaitStepInputAutomationStepInputSum(v)}
	case externalapi.ApplyTagStepInput:
		return externalapi.AutomationStepInput{OneOf: externalapi.NewApplyTagStepInputAutomationStepInputSum(v)}
	case externalapi.RemoveTagStepInput:
		return externalapi.AutomationStepInput{OneOf: externalapi.NewRemoveTagStepInputAutomationStepInputSum(v)}
	case externalapi.HttpStepInput:
		return externalapi.AutomationStepInput{OneOf: externalapi.NewHttpStepInputAutomationStepInputSum(v)}
	case externalapi.EndStepInput:
		return externalapi.AutomationStepInput{OneOf: externalapi.NewEndStepInputAutomationStepInputSum(v)}
	default:
		panic(fmt.Sprintf("testhelper: %T is not an external Step", st))
	}
}

// SiteTrigger is a site API Trigger on the Event action, with the filter given or none.
func SiteTrigger(action string, filter ...siteapi.EventFilterInput) siteapi.AutomationTriggerInput {
	ev := siteapi.EventTriggerInput{Kind: siteapi.EventTriggerInputKindEvent, Action: action}
	if len(filter) > 0 {
		ev.Filter = siteapi.NewOptEventFilterInput(filter[0])
	}
	return siteapi.AutomationTriggerInput{OneOf: siteapi.NewEventTriggerInputAutomationTriggerInputSum(ev)}
}

// ExternalTrigger is SiteTrigger for the /api surface.
func ExternalTrigger(action string, filter ...externalapi.EventFilterInput) externalapi.AutomationTriggerInput {
	ev := externalapi.EventTriggerInput{Kind: externalapi.EventTriggerInputKindEvent, Action: action}
	if len(filter) > 0 {
		ev.Filter = externalapi.NewOptEventFilterInput(filter[0])
	}
	return externalapi.AutomationTriggerInput{OneOf: externalapi.NewEventTriggerInputAutomationTriggerInputSum(ev)}
}

// SiteMatcher is an exit or goal event for a site API write: the action, and the filter when one is
// given (ADR 0133).
func SiteMatcher(action string, filter ...siteapi.EventFilterInput) siteapi.EventMatcherInput {
	m := siteapi.EventMatcherInput{Action: action}
	if len(filter) > 0 {
		m.Filter = siteapi.NewOptEventFilterInput(filter[0])
	}
	return m
}

// ExternalMatcher is SiteMatcher for the external API.
func ExternalMatcher(action string, filter ...externalapi.EventFilterInput) externalapi.EventMatcherInput {
	m := externalapi.EventMatcherInput{Action: action}
	if len(filter) > 0 {
		m.Filter = externalapi.NewOptEventFilterInput(filter[0])
	}
	return m
}

// JSONOfEach is the JSON array of the values, each encoded by its own MarshalJSON: a list of
// contract values read back, to compare with what was written.
func JSONOfEach[T any, PT interface {
	*T
	json.Marshaler
}](tb testing.TB, values []T) string {
	tb.Helper()
	raw := make([]json.RawMessage, len(values))
	for i := range values {
		raw[i] = json.RawMessage(JSONOf(tb, PT(&values[i])))
	}
	out, err := json.Marshal(raw)
	require.NoError(tb, err)
	return string(out)
}
