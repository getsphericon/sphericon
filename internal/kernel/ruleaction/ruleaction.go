// Package ruleaction is a Rule's Action as its version stores it (ADR 0169): the type of the
// document the rule_versions.actions column holds, which the Rules module validates, seals and runs.
package ruleaction

import (
	"fmt"

	"github.com/getsphericon/sphericon/internal/kernel/httpheader"
	"github.com/getsphericon/sphericon/internal/kernel/tagname"
)

// Type is the kind of an Action; its values are the contract's RuleActionType.
type Type string

// The Action types. Each has a Runner (rules.ActionRunners) that does it.
const (
	ApplyTag  Type = "apply_tag"
	RemoveTag Type = "remove_tag"
	// Enroll enrolls the Contact in an Automation (once ever, ADR 0046).
	Enroll Type = "enroll_automation"
	// HTTP calls a URL with a body rendered for the Contact and the Event.
	HTTP Type = "http"
)

// Action is one Action of a Rule as stored: its type and the parameters of that type.
type Action struct {
	Type Type `json:"type"`
	// Tag is the Tag name of apply_tag and remove_tag.
	Tag string `json:"tag,omitempty"`
	// AutomationID is the Automation enroll_automation enrolls the Contact in.
	AutomationID int64 `json:"automation_id,omitempty"`

	// An http Action (ADR 0039 shape, shared through internal/infra/httpcall) has ID, URL, Headers, Body
	// (a Liquid JSON template that sees the Contact and the `event`) and Secret. Stored, URL,
	// header values and Secret are ciphertext, sealed per field; in an Action being saved, an
	// empty one keeps the stored value of the Action of the same ID, and ClearSecret drops the
	// signing secret.
	ID          string              `json:"id,omitempty"`
	URL         string              `json:"url,omitempty"`
	Headers     []httpheader.Header `json:"headers,omitempty"`
	Body        string              `json:"body,omitempty"`
	Secret      string              `json:"secret,omitempty"`
	ClearSecret bool                `json:"-"`
}

// NamesTag reports whether the Action applies or removes the Tag.
func (a Action) NamesTag(name string) bool {
	return (a.Type == ApplyTag || a.Type == RemoveTag) && tagname.Same(a.Tag, name)
}

// EnrollsInto reports whether the Action enrolls Contacts in the Automation.
func (a Action) EnrollsInto(automationID int64) bool {
	return a.Type == Enroll && a.AutomationID == automationID
}

// Summary describes an Action in one line, without running it.
func (a Action) Summary() string {
	switch a.Type {
	case ApplyTag:
		return fmt.Sprintf("Apply tag %q", a.Tag)
	case RemoveTag:
		return fmt.Sprintf("Remove tag %q", a.Tag)
	case Enroll:
		return fmt.Sprintf("Enroll in automation %d", a.AutomationID)
	case HTTP:
		// The URL is stored sealed and may carry a secret: never described here.
		return "Call an HTTP endpoint"
	default:
		return string(a.Type)
	}
}
