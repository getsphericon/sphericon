// Package automationstep owns the stored Automation Step model and its interpretation.
// Authoring prepares and validates Steps before storing them; readers choose which version
// to interpret. This package neither selects versions nor executes Steps.
package automationstep

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
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
	KindBranch    Kind = "branch"
	KindSplit     Kind = "split"
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
// HTTP, End, Branch and Split.
type Payload interface {
	Kind() Kind
	// Stored is the payload as a version stores it: tag names, a Telegram step's image and
	// unsubscribe button trimmed, an SMS step's link tracking stored only when it is off, an email
	// step's editor version only with its document, and an HTTP step's ClearSecret dropped.
	Stored() Payload
}

// Sending is a send Step, Email, Telegram or SMS: what the three share.
type Sending interface {
	Payload
	// Integration is the Integration it sends through, nil for its Channel's default (ADR 0081).
	Integration() *int64
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
	if p, ok := s.Payload.(Sending); ok {
		return p.Integration()
	}
	return nil
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

// Tagging is a Step that applies or removes a Tag, ApplyTag or RemoveTag: what the two share.
type Tagging interface {
	Payload
	// TagName is the name of the Tag the Step applies or removes.
	TagName() string
}

func (p ApplyTag) TagName() string  { return p.Tag }
func (p RemoveTag) TagName() string { return p.Tag }

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

// Branch sends an Enrollment down the first of its Paths whose Condition holds on the Contact's
// live data when it reaches the Branch, down Otherwise when none does, and down Unknown when the
// first Condition that is not false is unknown (ADR 0124). Its paths rejoin at the Step after it.
type Branch struct {
	Paths     []BranchPath `json:"paths"`
	Otherwise Path         `json:"otherwise"`
	Unknown   *Path        `json:"unknown,omitempty"`
}

// BranchPath is a listed path of a Branch: its id, the Condition that sends an Enrollment down it
// and its Steps.
type BranchPath struct {
	ID        ID                   `json:"id"`
	Condition conditiontree.Branch `json:"condition"`
	Steps     []Step               `json:"steps"`
}

// Fork is a Step whose paths rejoin after it (ADR 0122, ADR 0124): the walker reaches the Steps of
// its paths through it, and a new fork kind implements it.
type Fork interface {
	Payload
	// Forks are the fork's paths, in the order the walker visits them: a Branch's listed paths,
	// then otherwise, then unknown when it has one.
	Forks() []Path
	// WithForks is the fork with its paths replaced by paths, given in the order Forks gives them;
	// what else it holds (a path's Condition) is kept.
	WithForks(paths []Path) Fork
}

func (b Branch) Forks() []Path {
	out := make([]Path, 0, len(b.Paths)+2)
	for _, p := range b.Paths {
		out = append(out, Path{ID: p.ID, Steps: p.Steps})
	}
	out = append(out, b.Otherwise)
	if b.Unknown != nil {
		out = append(out, *b.Unknown)
	}
	return out
}

func (b Branch) WithForks(paths []Path) Fork {
	listed := make([]BranchPath, len(b.Paths))
	for i, p := range b.Paths {
		listed[i] = BranchPath{ID: paths[i].ID, Condition: p.Condition, Steps: paths[i].Steps}
	}
	out := Branch{Paths: listed, Otherwise: paths[len(listed)]}
	if b.Unknown != nil {
		unknown := paths[len(listed)+1]
		out.Unknown = &unknown
	}
	return out
}

// Split sends an Enrollment down one of its Paths by weight (ADR 0124): a draw of its Contact, its
// Step and its ordinal (Draw) lands on a path by the running total of the weights (At). An empty
// path is a holdout, sent nothing. Its paths rejoin at the Step after it.
type Split struct {
	Paths []SplitPath `json:"paths"`
}

// SplitPath is a path of a Split: its id, its weight, a whole share of 100, and its Steps.
type SplitPath struct {
	ID     ID     `json:"id"`
	Weight int    `json:"weight"`
	Steps  []Step `json:"steps"`
}

func (s Split) Forks() []Path {
	out := make([]Path, len(s.Paths))
	for i, p := range s.Paths {
		out[i] = Path{ID: p.ID, Steps: p.Steps}
	}
	return out
}

func (s Split) WithForks(paths []Path) Fork {
	out := make([]SplitPath, len(s.Paths))
	for i, p := range s.Paths {
		out[i] = SplitPath{ID: paths[i].ID, Weight: p.Weight, Steps: paths[i].Steps}
	}
	return Split{Paths: out}
}

// At is the path a draw of 0 to 99 lands on: the first whose running total of the weights exceeds
// it. The save holds the weights to whole numbers of at least 1 summing to 100, so every draw lands
// on a path; a draw past the total lands on the last path.
func (s Split) At(draw int) ID {
	total := 0
	for _, p := range s.Paths {
		total += p.Weight
		if draw < total {
			return p.ID
		}
	}
	if len(s.Paths) == 0 {
		return ""
	}
	return s.Paths[len(s.Paths)-1].ID
}

// Draw is the draw, 0 to 99, of the Contact at the Split's Step for the Enrollment of the ordinal
// (ADR 0124, ADR 0127): the first eight bytes of the SHA-256 of "<contact>:<step>:<ordinal>",
// big-endian, mod 100. It holds no random state, so the same Enrollment always draws the same, and
// a new Enrollment of the Contact draws again.
func Draw(contactID int64, step ID, ordinal int) int {
	sum := sha256.Sum256(fmt.Appendf(nil, "%d:%s:%d", contactID, step, ordinal))
	return int(binary.BigEndian.Uint64(sum[:8]) % 100)
}

func (p Email) Integration() *int64    { return p.IntegrationID }
func (p Telegram) Integration() *int64 { return p.IntegrationID }
func (s SMS) Integration() *int64      { return s.IntegrationID }

func (p Email) Stored() Payload {
	if len(p.Doc) == 0 {
		p.Doc, p.EditorVersion = nil, ""
	}
	return p
}

func (p Telegram) Stored() Payload {
	p.ImageURL, p.UnsubscribeButton = strings.TrimSpace(p.ImageURL), strings.TrimSpace(p.UnsubscribeButton)
	return p
}

func (s SMS) Stored() Payload {
	if s.TracksLinks() {
		s.LinkTracking = nil // on is the default, stored as nothing
	}
	return s
}

func (p Wait) Stored() Payload      { return p }
func (p ApplyTag) Stored() Payload  { return ApplyTag{Tag: strings.TrimSpace(p.Tag)} }
func (p RemoveTag) Stored() Payload { return RemoveTag{Tag: strings.TrimSpace(p.Tag)} }
func (p HTTP) Stored() Payload {
	return HTTP{URL: p.URL, Headers: p.Headers, Body: p.Body, Secret: p.Secret}
}
func (p End) Stored() Payload { return p }

// Stored is the Branch as it is: the walker stores the Steps of its paths (Rewrite).
func (b Branch) Stored() Payload { return b }

// Stored is the Split as it is: the walker stores the Steps of its paths (Rewrite).
func (s Split) Stored() Payload { return s }

func (Email) Kind() Kind     { return KindEmail }
func (Telegram) Kind() Kind  { return KindTelegram }
func (SMS) Kind() Kind       { return KindSMS }
func (Wait) Kind() Kind      { return KindWait }
func (ApplyTag) Kind() Kind  { return KindApplyTag }
func (RemoveTag) Kind() Kind { return KindRemoveTag }
func (HTTP) Kind() Kind      { return KindHTTP }
func (End) Kind() Kind       { return KindEnd }
func (Branch) Kind() Kind    { return KindBranch }
func (Split) Kind() Kind     { return KindSplit }

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
	KindBranch:    decodeAs[Branch],
	KindSplit:     decodeAs[Split],
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
// another Step or path of the version holds too. Identify returns it as a *BadIDError.
var ErrBadID = errors.New("automationstep: bad id")

// BadIDError is a refused id: Detail names the Step by its id and kind, or the path, and says
// why. It is ErrBadID (errors.Is).
type BadIDError struct {
	Detail string
}

func (e *BadIDError) Error() string { return ErrBadID.Error() + ": " + e.Detail }

func (e *BadIDError) Unwrap() error { return ErrBadID }

// NewID mints the id of a Step or a path.
func NewID() ID { return ID(uuid.NewString()) }

// Identify gives the root path, each Step of the tree and each path of its forks that has no id
// one from mint, and keeps an id it holds. An id that is not a UUID in its lowercase text form, or that two of them share, is a
// *BadIDError naming the Step by its id and kind, or the path. The path given is left as it was.
func Identify(p Path, mint func() ID) (Path, error) {
	seen := map[ID]bool{}
	// claim keeps or mints the id of the Step st, or of the path for nil.
	claim := func(id ID, st *Step) (ID, error) {
		if id == "" {
			id = mint()
		}
		name := func(id string) string {
			if st == nil {
				return "the path " + id
			}
			return fmt.Sprintf("step %s (%s)", id, st.Kind())
		}
		if parsed, err := uuid.Parse(string(id)); err != nil || parsed.String() != string(id) {
			return "", &BadIDError{Detail: name(strconv.Quote(string(id))) + ": the id is not a lowercase UUID"}
		}
		if seen[id] {
			return "", &BadIDError{Detail: name(string(id)) + ": the id is used twice in the version"}
		}
		seen[id] = true
		return id, nil
	}
	root, err := claim(p.ID, nil)
	if err != nil {
		return Path{}, err
	}
	out, err := Map(p, func(_ int, st *Step) (Step, error) {
		id, err := claim(st.ID, st)
		if err != nil {
			return Step{}, err
		}
		f, ok := st.Payload.(Fork)
		if !ok {
			return Step{ID: id, Payload: st.Payload}, nil
		}
		// A fork's paths are claimed after the fork and before their Steps, which Map reaches next.
		paths := f.Forks()
		for i := range paths {
			if paths[i].ID, err = claim(paths[i].ID, nil); err != nil {
				return Step{}, err
			}
		}
		return Step{ID: id, Payload: f.WithForks(paths)}, nil
	})
	if err != nil {
		return Path{}, err
	}
	out.ID = root
	return out, nil
}
