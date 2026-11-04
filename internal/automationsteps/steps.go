// Package automationsteps owns the stored Automation Step model and its interpretation.
// Authoring prepares and validates Steps before storing them; readers choose which version
// to interpret. This package neither selects versions nor executes Steps.
package automationsteps

import (
	"encoding/json"
	"slices"

	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/telegramrender"
)

// Kind is the kind of a Step.
type Kind string

// Step kinds.
const (
	Email     Kind = "email"
	Telegram  Kind = "telegram"
	SMS       Kind = "sms"
	Wait      Kind = "wait"
	ApplyTag  Kind = "apply_tag"
	RemoveTag Kind = "remove_tag"
	HTTP      Kind = "http"
)

// Channel is the Channel a send step of the kind sends on (ADR 0081: the step's kind fixes its
// Channel); false for a kind that sends nothing. A new Channel's send step adds its kind here, and
// Publish, the step executor and the reports take it up.
func (k Kind) Channel() (messaging.Channel, bool) {
	switch k {
	case Email:
		return messaging.ChannelEmail, true
	case Telegram:
		return messaging.ChannelTelegram, true
	case SMS:
		return messaging.ChannelSMS, true
	default:
		return "", false
	}
}

// Step is one node of an Automation's linear sequence, in the executor's on-disk
// JSON form. Email steps carry Subject and Body (HTML), wait steps Seconds, tag
// steps Tag (the Tag name).
type Step struct {
	Type Kind `json:"type"`
	// IntegrationID is the Integration a send step sends through, of the step's Channel; nil is
	// that Channel's default (ADR 0081).
	IntegrationID *int64 `json:"integrationId,omitempty"`
	Subject       string `json:"subject,omitempty"`
	Body          string `json:"body,omitempty"`
	// Doc is the editor's JSON document an email step's Body was built from, and EditorVersion
	// the editor that rendered it (ADR 0040). A step written as HTML has neither. A step being
	// saved carries a Doc or a Body, not both: the save renders the Doc to the Body.
	Doc           json.RawMessage `json:"doc,omitempty"`
	EditorVersion string          `json:"editorVersion,omitempty"`
	Seconds       int             `json:"seconds,omitempty"`
	Tag           string          `json:"tag,omitempty"`
	// Text is a Telegram step's message: Telegram's HTML subset with Liquid merge tags. With an
	// ImageURL it is the image's caption. On an SMS step it is the text: plain, with Liquid merge
	// tags.
	Text string `json:"text,omitempty"`
	// UnsubscribeButton is the label of a Telegram step's unsubscribe button, "" for none.
	UnsubscribeButton string `json:"unsubscribeButton,omitempty"`
	// ImageURL is the https URL of a Telegram step's image, "" for none.
	ImageURL string `json:"imageUrl,omitempty"`
	// Buttons are a Telegram step's link buttons, one per row, at most telegramrender.MaxButtons.
	Buttons []telegramrender.Button `json:"buttons,omitempty"`
	// LinkPreview is whether a link in a Telegram step's text unfolds a preview; nil is on.
	LinkPreview *bool `json:"linkPreview,omitempty"`
	// LinkTracking is whether the links of an SMS step's text go out as short tracked links (ADR
	// 0083); nil is on.
	LinkTracking *bool `json:"linkTracking,omitempty"`

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

// TracksLinks reports whether the links of an SMS step go out as short tracked links: unless the
// step turned it off.
func (s *Step) TracksLinks() bool { return s.LinkTracking == nil || *s.LinkTracking }

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
