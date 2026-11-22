// Package automationstep owns the stored Automation Step model and its interpretation.
// Authoring prepares and validates Steps before storing them; readers choose which version
// to interpret. This package neither selects versions nor executes Steps.
package automationstep

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/google/uuid"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/httpheader"
	"github.com/getsphericon/sphericon/internal/kernel/telegramrender"
)

// Kind is the kind of a Step: the `type` it is stored and shown under.
type Kind string

// Step kinds.
const (
	KindEmail     Kind = "email"
	KindTelegram  Kind = "telegram"
	KindSMS       Kind = "sms"
	KindWait      Kind = "wait"
	KindApplyTag  Kind = "apply_tag"
	KindRemoveTag Kind = "remove_tag"
	KindHTTP      Kind = "http"
	KindEnd       Kind = "end"
)

// Channel is the Channel a send step of the kind sends on (ADR 0081: the step's kind fixes its
// Channel); false for a kind that sends nothing. A new Channel's send step adds its kind here, and
// Publish, the step executor and the reports take it up.
func (k Kind) Channel() (channels.Channel, bool) {
	switch k {
	case KindEmail:
		return channels.Email, true
	case KindTelegram:
		return channels.Telegram, true
	case KindSMS:
		return channels.SMS, true
	default:
		return "", false
	}
}

// ID is the id of a Step or a path, which every version keeps (ADR 0122): an Enrollment, a job, a
// Step record and an Automation message name a Step by it. It is a UUID in its lowercase text form.
type ID string

// Path is a path of Steps, in the order an Enrollment walks them, with the id every version keeps
// (ADR 0122, ADR 0124). A version's Steps are its root path: the type of the document the
// automation_versions.steps column holds (ADR 0169).
type Path struct {
	ID    ID     `json:"id"`
	Steps []Step `json:"steps"`
}

// Root is a path of the Steps given, with no id yet: Identify gives it one.
func Root(steps ...Step) Path {
	if steps == nil {
		steps = []Step{}
	}
	return Path{Steps: steps}
}

// Step is one Step of a path: its id, which every version keeps, and what it does, a payload of
// its kind's own type. It is stored as one object, the id and the kind (`type`) beside the fields
// of its kind; a field of another kind is refused when it is read.
type Step struct {
	ID      ID
	Payload Payload
}

// New is a Step with no id yet: Identify gives it one.
func New(p Payload) Step { return Step{Payload: p} }

// Payload is what a Step does, one type per kind: Email, Telegram, SMS, Wait, ApplyTag, RemoveTag,
// HTTP and End.
type Payload interface {
	Kind() Kind
}

// Kind is the Step's kind; "" for a Step with no payload.
func (s Step) Kind() Kind {
	if s.Payload == nil {
		return ""
	}
	return s.Payload.Kind()
}

// Integration is the Integration a send step sends through, nil for its Channel's default and for
// a Step that sends nothing.
func (s Step) Integration() *int64 {
	switch p := s.Payload.(type) {
	case Email:
		return p.IntegrationID
	case Telegram:
		return p.IntegrationID
	case SMS:
		return p.IntegrationID
	default:
		return nil
	}
}

// Email sends an email. Body is HTML. Doc is the editor's JSON document the Body was built from,
// and EditorVersion the editor that rendered it (ADR 0040); a step written as HTML has neither. A
// step being saved carries a Doc or a Body, not both: the save renders the Doc to the Body.
type Email struct {
	// IntegrationID is the Integration it sends through, of the email Channel; nil is that
	// Channel's default (ADR 0081).
	IntegrationID *int64          `json:"integrationId,omitempty"`
	Subject       string          `json:"subject"`
	Body          string          `json:"body"`
	Doc           json.RawMessage `json:"doc,omitempty"`
	EditorVersion string          `json:"editorVersion,omitempty"`
}

// Telegram sends a Telegram message. Text is Telegram's HTML subset with Liquid merge tags; with
// an ImageURL it is the image's caption.
type Telegram struct {
	IntegrationID *int64 `json:"integrationId,omitempty"`
	Text          string `json:"text"`
	// UnsubscribeButton is the label of the unsubscribe button, "" for none.
	UnsubscribeButton string `json:"unsubscribeButton,omitempty"`
	// ImageURL is the https URL of the image, "" for none.
	ImageURL string `json:"imageUrl,omitempty"`
	// Buttons are the link buttons, one per row, at most bounds.TelegramButtonsMaxItems.
	Buttons []telegramrender.Button `json:"buttons,omitempty"`
	// LinkPreview is whether a link in the text unfolds a preview; nil is on.
	LinkPreview *bool `json:"linkPreview,omitempty"`
}

// SMS sends a text: plain, with Liquid merge tags.
type SMS struct {
	IntegrationID *int64 `json:"integrationId,omitempty"`
	Text          string `json:"text"`
	// LinkTracking is whether the links of the text go out as short tracked links (ADR 0083); nil
	// is on.
	LinkTracking *bool `json:"linkTracking,omitempty"`
}

// TracksLinks reports whether the links of the text go out as short tracked links: unless the
// step turned it off.
func (s SMS) TracksLinks() bool { return s.LinkTracking == nil || *s.LinkTracking }

// Wait holds the Enrollment for Seconds before the next Step.
type Wait struct {
	Seconds int `json:"seconds"`
}

// ApplyTag applies the Tag named Tag to the enrolled Contact.
type ApplyTag struct {
	Tag string `json:"tag"`
}

// RemoveTag removes the Tag named Tag from the enrolled Contact.
type RemoveTag struct {
	Tag string `json:"tag"`
}

// HTTP calls a hook (ADR 0039) with a Liquid JSON Body. Stored, URL, header values and Secret are
// ciphertext, sealed per field; in a step being saved, an empty one keeps the stored value of the
// step with the same id, and ClearSecret drops the signing secret.
type HTTP struct {
	URL         string              `json:"url,omitempty"`
	Headers     []httpheader.Header `json:"headers,omitempty"`
	Body        string              `json:"body"`
	Secret      string              `json:"secret,omitempty"`
	ClearSecret bool                `json:"-"`
}

// End finishes its path and ends the Enrollment as completed at once (ADR 0124): a path stops early
// by design, which is no exit. It holds nothing but its id, and it is the last Step of its path.
type End struct{}

func (Email) Kind() Kind     { return KindEmail }
func (Telegram) Kind() Kind  { return KindTelegram }
func (SMS) Kind() Kind       { return KindSMS }
func (Wait) Kind() Kind      { return KindWait }
func (ApplyTag) Kind() Kind  { return KindApplyTag }
func (RemoveTag) Kind() Kind { return KindRemoveTag }
func (HTTP) Kind() Kind      { return KindHTTP }
func (End) Kind() Kind       { return KindEnd }

// decoders read the fields of each kind's payload, refusing any field the kind does not hold.
var decoders = map[Kind]func([]byte) (Payload, error){
	KindEmail:     decodeAs[Email],
	KindTelegram:  decodeAs[Telegram],
	KindSMS:       decodeAs[SMS],
	KindWait:      decodeAs[Wait],
	KindApplyTag:  decodeAs[ApplyTag],
	KindRemoveTag: decodeAs[RemoveTag],
	KindHTTP:      decodeAs[HTTP],
	KindEnd:       decodeAs[End],
}

func decodeAs[P Payload](b []byte) (Payload, error) {
	var p P
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return nil, err
	}
	return p, nil
}

// MarshalJSON writes the Step as one object: its id and kind beside its payload's fields.
func (s Step) MarshalJSON() ([]byte, error) {
	if s.Payload == nil {
		return nil, errors.New("automationstep: a step without a payload")
	}
	b, err := json.Marshal(s.Payload)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	if fields["id"], err = json.Marshal(s.ID); err != nil {
		return nil, err
	}
	if fields["type"], err = json.Marshal(s.Payload.Kind()); err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

// UnmarshalJSON reads a Step written by MarshalJSON. A kind nobody knows, and a field the Step's
// kind does not hold, are errors: the stored payload is closed.
func (s *Step) UnmarshalJSON(b []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	var id ID
	var kind Kind
	if raw, ok := fields["id"]; ok {
		if err := json.Unmarshal(raw, &id); err != nil {
			return fmt.Errorf("step id: %w", err)
		}
	}
	if err := json.Unmarshal(fields["type"], &kind); err != nil {
		return fmt.Errorf("step type: %w", err)
	}
	decode, ok := decoders[kind]
	if !ok {
		return fmt.Errorf("step type %q is unknown", kind)
	}
	rest := maps.Clone(fields)
	delete(rest, "id")
	delete(rest, "type")
	payload, err := json.Marshal(rest)
	if err != nil {
		return err
	}
	p, err := decode(payload)
	if err != nil {
		return fmt.Errorf("%s step: %w", kind, err)
	}
	*s = Step{ID: id, Payload: p}
	return nil
}

// SendsEmail reports whether the path contains an email Step: steps with none have no email
// sending dependency.
func SendsEmail(p Path) bool {
	for _, st := range All(p) {
		if st.Kind() == KindEmail {
			return true
		}
	}
	return false
}

// ErrBadID is a Step's or a path's id that is not a UUID in its lowercase text form, or that
// another Step or path of the version holds too.
var ErrBadID = errors.New("automationstep: bad id")

// NewID mints the id of a Step or a path.
func NewID() ID { return ID(uuid.NewString()) }

// Identify gives the path, and each of its Steps, that has no id one from mint, and keeps an id
// it holds. An id that is not a UUID in its lowercase text form, or that two of them share, is
// ErrBadID, naming the Step (1-based) or the path. The path given is left as it was.
func Identify(p Path, mint func() ID) (Path, error) {
	seen := map[ID]bool{}
	claim := func(id ID, what string) (ID, error) {
		if id == "" {
			id = mint()
		}
		if parsed, err := uuid.Parse(string(id)); err != nil || parsed.String() != string(id) {
			return "", fmt.Errorf("%w: %s: %q is not a lowercase UUID", ErrBadID, what, id)
		}
		if seen[id] {
			return "", fmt.Errorf("%w: %s: id %s is used twice in the version", ErrBadID, what, id)
		}
		seen[id] = true
		return id, nil
	}
	root, err := claim(p.ID, "the path")
	if err != nil {
		return Path{}, err
	}
	out, err := Map(p, func(pos int, st *Step) (Step, error) {
		id, err := claim(st.ID, fmt.Sprintf("step %d", pos+1))
		return Step{ID: id, Payload: st.Payload}, err
	})
	if err != nil {
		return Path{}, err
	}
	out.ID = root
	return out, nil
}
