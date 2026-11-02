// Package automationsteps owns the stored Automation Step model and its interpretation.
// Authoring prepares and validates Steps before storing them; readers choose which version
// to interpret. This package neither selects versions nor executes Steps.
package automationsteps

import (
	"encoding/json"
	"slices"

	"github.com/getsphericon/sphericon/internal/httpcall"
)

// Kind is the kind of a Step.
type Kind string

// Step kinds.
const (
	Email     Kind = "email"
	Wait      Kind = "wait"
	ApplyTag  Kind = "apply_tag"
	RemoveTag Kind = "remove_tag"
	HTTP      Kind = "http"
)

// Step is one node of an Automation's linear sequence, in the executor's on-disk
// JSON form. Email steps carry Subject and Body (HTML), wait steps Seconds, tag
// steps Tag (the Tag name).
type Step struct {
	Type    Kind   `json:"type"`
	Subject string `json:"subject,omitempty"`
	Body    string `json:"body,omitempty"`
	// Doc is the editor's JSON document an email step's Body was built from, and EditorVersion
	// the editor that rendered it (ADR 0040). A step written as HTML has neither. A step being
	// saved carries a Doc or a Body, not both: the save renders the Doc to the Body.
	Doc           json.RawMessage `json:"doc,omitempty"`
	EditorVersion string          `json:"editorVersion,omitempty"`
	Seconds       int             `json:"seconds,omitempty"`
	Tag           string          `json:"tag,omitempty"`

	// HTTP steps (ADR 0039) hold ID, URL, Headers, Body (a Liquid JSON template) and
	// Secret. Stored, URL, header values and Secret are ciphertext, sealed per field;
	// in a step being saved, an empty one keeps the stored value of the step with the
	// same ID, and ClearSecret drops the signing secret.
	ID          string            `json:"id,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     []httpcall.Header `json:"headers,omitempty"`
	Secret      string            `json:"secret,omitempty"`
	ClearSecret bool              `json:"-"`
}

// Decode parses a stored definition. An empty definition has no steps.
func Decode(definition string) ([]Step, error) {
	if definition == "" {
		return nil, nil
	}
	var steps []Step
	if err := json.Unmarshal([]byte(definition), &steps); err != nil {
		return nil, err
	}
	return steps, nil
}

// SendsEmail reports whether a stored definition contains an email Step. A definition with
// no email Step has no email sending dependency; an unreadable definition cannot establish that.
func SendsEmail(definition string) (bool, error) {
	steps, err := Decode(definition)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(steps, func(step Step) bool { return step.Type == Email }), nil
}
