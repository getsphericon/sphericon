// Package preset is the Preset engine (ADR 0037, ADR 0076): a Preset describes one provider's
// inbound callbacks, how a request is verified and how its payload becomes the closed set of
// inbound facts: Delivery notices, Inbound messages and Reachability changes. The engine does no
// I/O and reads no meaning into a fact (that is the core's, per Channel): it takes a Preset, the
// raw request and the Integration's params and returns the validated facts, an optional action
// for the caller to perform, or a typed error.
package preset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/santhosh-tekuri/jsonschema/v6"

	kernelchannels "github.com/getsphericon/sphericon/internal/kernel/channels"
)

// Limits on what one request may make the engine do.
const (
	// MaxBodyBytes is the largest request body the engine parses.
	MaxBodyBytes = 1 << 20
	// MaxProgramBytes is the largest transform program.
	MaxProgramBytes = 16 << 10
	// MaxFacts is the most facts (of every kind together) one request may yield.
	// Provider preset cap on facts per request; not in the contract.
	MaxFacts = 100
	// TransformTimeout bounds one run of a transform.
	TransformTimeout = 100 * time.Millisecond
)

// The ways a request is refused. A caller answers 4xx for the first three: the request
// is the provider's to fix, a retry would not change it. ErrUnavailable is ours: 5xx.
var (
	// ErrUnauthentic: the request did not come from the provider for this Integration.
	ErrUnauthentic = errors.New("preset: request is not authentic")
	// ErrMalformed: the body is not JSON (not form-encoded, for a form-encoded request), or is
	// over the limit.
	ErrMalformed = errors.New("preset: request is malformed")
	// ErrTransformFailed: the transform failed, timed out or returned something that is
	// not a list of valid facts.
	ErrTransformFailed = errors.New("preset: transform failed")
	// ErrUnavailable: the request could not be verified because something on our side
	// failed (the signing certificate could not be fetched). It says nothing about the
	// request, so a caller answers 5xx and the provider retries.
	ErrUnavailable = errors.New("preset: verification is unavailable")
)

// Kind of a Delivery notice.
type Kind string

const (
	Bounce    Kind = "bounce"
	Complaint Kind = "complaint"
	Delivery  Kind = "delivery"
	Delay     Kind = "delay"
	Drop      Kind = "drop"
	// Undelivered: the message did not reach the person, on a Channel other than email (an SMS the
	// carrier could not deliver, ADR 0079). Email says bounced instead.
	Undelivered Kind = "undelivered"
)

// MaxReasonChars caps a notice's provider reason, in characters.
// Cap of the reason an email delivery Event keeps; not in the contract.
const MaxReasonChars = 500

// DeliveryNotice is the normalized shape of a delivery result, one of the facts a Preset
// produces. Only a hard
// bounce or a complaint enters Suppression; a soft bounce still counts in the
// deliverability rates.
type DeliveryNotice struct {
	// ID is the provider's event id; with Destination it dedupes a redelivery.
	ID string `json:"id"`
	// Channel is the Channel of the message the notice is about: email when the transform names
	// none.
	Channel Channel `json:"channel"`
	// Destination is where the message went: on email the normalized (trimmed, lower-cased)
	// address, on another Channel the trimmed value the provider sent (a phone number, as the
	// provider wrote it; the core normalizes it).
	Destination string `json:"destination"`
	Kind        Kind   `json:"kind"`
	// Hard is meaningful for a bounce only: a soft bounce is the temporary kind.
	Hard bool `json:"hard"`
	// SendingDomain is the lower-cased domain the message was sent from, "" when unknown.
	SendingDomain string `json:"sending_domain,omitempty"`
	// MessageID is the provider's id of the message the notice is about, "" when the
	// provider returns none: it links the notice to its Outbound message.
	MessageID string `json:"message_id,omitempty"`
	// Reason is the provider's short explanation of a bounce, delay, drop or undelivered, capped at
	// MaxReasonChars characters.
	Reason string `json:"reason,omitempty"`
	// Status is the provider's own word for the state it reports (Twilio's MessageStatus), ""
	// when it sends none: kept as it came, so the core's reading of it loses nothing.
	Status string `json:"status,omitempty"`
	// ErrorCode is the provider's error code for a failure (Twilio's ErrorCode), "" when none.
	ErrorCode string `json:"error_code,omitempty"`
}

// Channel is the Channel an Inbound message came in on: one of the Channels of ADR 0074.
type Channel string

// channels are the Channels of ADR 0074. A Channel sphericon does not build yet is still read
// here: the Ingest hook drops a message whose Channel is not its Integration's.
// The Channels sphericon builds are the kernel's; the others are named here until they are built.
var channels = []Channel{
	Channel(kernelchannels.Email), Channel(kernelchannels.SMS), "whatsapp", Channel(kernelchannels.Telegram),
	"web_push", "mobile_push",
}

// The kinds a transform names the two facts by that are not Delivery notices; a Delivery notice
// is named by its own Kind. `sphericon preset test` prints a fact under the same kind.
const (
	KindInboundMessage     = "inbound_message"
	KindReachabilityChange = "reachability_change"
)

// Limits of an Inbound message's free text. Longer values are cut, not refused: the person
// wrote them, and refusing would have the provider redeliver the request forever.
const (
	// MaxTextChars caps an Inbound message's text, in characters (Telegram's message limit).
	// Cap of the text a provider preset reads from an inbound message; not in the contract.
	MaxTextChars = 4096
	// MaxPayloadChars caps the payload of a pressed button, in characters.
	// Cap of a button payload a provider preset reads; not in the contract.
	MaxPayloadChars = 1024
)

// InboundMessage is what a person sent on a Channel, as the Preset parsed it (ADR 0076): a text
// they wrote or the payload of a button they pressed. What it means (a `/start` token, a STOP
// keyword) is read by the core, per Channel, never by the Preset.
type InboundMessage struct {
	// ID is the provider's event id: the core keys its deduplication on it (with the Channel and
	// the Integration), since a provider redelivers what it was not sure was received.
	ID      string  `json:"id"`
	Channel Channel `json:"channel"`
	// Destination is the address the message came from, trimmed: on Telegram the user id.
	Destination string `json:"destination"`
	// Country is the country the provider says the sender is in (ISO 3166-1 alpha-2), where it
	// says one: a phone number written without its country code is read with it.
	Country string `json:"country,omitempty"`
	// ChatID is the chat the message came in, where the Channel has one: a reply goes there.
	ChatID string `json:"chat_id,omitempty"`
	// Text is what the person wrote, as written, cut at MaxTextChars characters.
	Text string `json:"text,omitempty"`
	// Payload is the payload of a pressed button, cut at MaxPayloadChars characters.
	Payload string `json:"payload,omitempty"`
	// MessageID is the provider's id of the message: of the one pressed on, for a button.
	MessageID string `json:"message_id,omitempty"`
	// CallbackID is the provider's id of the button press, which the provider wants answered
	// (Telegram's callback query id, for answerCallbackQuery): "" for a written message.
	CallbackID string `json:"callback_id,omitempty"`
}

// Unreachability is why a Destination can no longer be reached.
type Unreachability string

const (
	// Blocked: the person blocked the sender (a bot), and may unblock it.
	Blocked Unreachability = "blocked"
	// Deactivated: the person's account is gone.
	Deactivated Unreachability = "deactivated"
)

// ReachabilityChange is a provider saying on its own that a Destination can or can no longer be
// reached (ADR 0076). Reason is set exactly when it can no longer be.
type ReachabilityChange struct {
	// ID is the provider's event id, as on an Inbound message.
	ID          string         `json:"id"`
	Destination string         `json:"destination"`
	Reachable   bool           `json:"reachable"`
	Reason      Unreachability `json:"reason,omitempty"`
}

// ActionKind is something the caller performs for the engine.
type ActionKind string

// ActionConfirm means fetch URL to confirm a subscription (SNS).
const ActionConfirm ActionKind = "confirm"

// Action is a side effect the engine asks of its caller, which performs it through the
// SSRF-guarded client.
type Action struct {
	Kind ActionKind
	URL  string
}

// Result of a verified request: the facts it carried, each kind in the order the transform
// returned it, and the action asked of the caller.
type Result struct {
	Notices             []DeliveryNotice
	InboundMessages     []InboundMessage
	ReachabilityChanges []ReachabilityChange
	Action              *Action
}

// Empty reports whether the request carried no fact.
func (r Result) Empty() bool {
	return len(r.Notices) == 0 && len(r.InboundMessages) == 0 && len(r.ReachabilityChanges) == 0
}

// Params are the values the Owner entered on the Integration, validated against the
// Preset's params schema. A secret is a string, or a list of strings (the current one
// first, then the one being retired) while it rotates.
type Params map[string]any

// Request is what arrived at the hook.
type Request struct {
	// URL is the full URL the request was posted to (scheme, host, path and query), as the
	// provider addressed it: a scheme that signs the URL (twilio) checks it.
	URL    string
	Header http.Header
	// Body is the raw body, exactly as received.
	Body []byte
}

// Preset is a parsed, validated and compiled Preset file.
type Preset struct {
	name        string
	description string
	params      map[string]any
	paramsDoc   *jsonschema.Schema
	verifier    verifier
	transform   *gojq.Code
	tests       []testCase
}

// Name is the Preset's name.
func (p *Preset) Name() string { return p.name }

// Description is the Preset's one-line description, possibly empty.
func (p *Preset) Description() string { return p.description }

// ParamsSchema is the JSON Schema of the params, as the Preset file wrote it.
func (p *Preset) ParamsSchema() map[string]any { return p.params }

// SecretParams names the params the schema marks `writeOnly`: secrets, which are stored
// encrypted, never returned by a read and rotated rather than replaced.
func (p *Preset) SecretParams() []string {
	props, _ := p.params["properties"].(map[string]any)
	var names []string
	for name, def := range props {
		if d, ok := def.(map[string]any); ok && d["writeOnly"] == true {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// ValidateParams checks the Integration's values against the params schema.
func (p *Preset) ValidateParams(values Params) error {
	doc, err := jsonValue(map[string]any(values))
	if err != nil {
		return err
	}
	if err := p.paramsDoc.Validate(doc); err != nil {
		return fmt.Errorf("params of preset %q: %w", p.name, err)
	}
	return nil
}

// Handle verifies the request and turns its payload into facts. Nothing
// reaches the transform unverified: a header scheme checks the raw bytes before any
// parsing, a body scheme parses under the body limit, verifies, and only then
// transforms. now is the clock, for timestamp tolerance.
func (p *Preset) Handle(ctx context.Context, req Request, params Params, now time.Time) (Result, error) {
	if len(req.Body) > MaxBodyBytes {
		return Result{}, fmt.Errorf("%w: body is over %d bytes", ErrMalformed, MaxBodyBytes)
	}
	var body any
	if p.verifier.overBody() {
		var err error
		if body, err = parseBody(req); err != nil {
			return Result{}, err
		}
		if err := p.verifier.verify(ctx, req, body, params, now); err != nil {
			return Result{}, refusal(err)
		}
	} else {
		if err := p.verifier.verify(ctx, req, nil, params, now); err != nil {
			return Result{}, refusal(err)
		}
		var err error
		if body, err = parseBody(req); err != nil {
			return Result{}, err
		}
	}
	// Telegram's built-in Preset stringifies update_id for every fact. Validate the provider's
	// input first, so a missing id never becomes the string "null" or another fabricated identity.
	if p.name == BuiltinTelegram {
		if err := validateTelegramUpdate(body); err != nil {
			return Result{}, err
		}
	}
	res, err := p.run(ctx, body, req.Header)
	if err != nil {
		return Result{}, err
	}
	if a, ok := p.verifier.(interface{ action(body any) *Action }); ok {
		res.Action = a.action(body)
	}
	return res, nil
}

// refusal is what a failed verification becomes: unauthentic, unless the verifier says
// the notice was on our side.
func refusal(err error) error {
	if errors.Is(err, ErrUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrUnauthentic, err)
}

// forTest is the Preset as its own tests run it: offline, so an `sns` Preset checks
// signatures against the test certificate instead of fetching one.
func (p *Preset) forTest() *Preset {
	q := *p
	if s, ok := p.verifier.(*snsVerifier); ok {
		q.verifier = &snsVerifier{c: s.c, certs: TestSNSCertificates}
	}
	return &q
}

// formContentType is the media type of a form-encoded body (Twilio's callbacks).
const formContentType = "application/x-www-form-urlencoded"

// parseBody reads the body a transform sees: a form-encoded request's parameters, and JSON
// whatever any other Content-Type says.
func parseBody(req Request) (any, error) {
	if isForm(req.Header) {
		return parseForm(req.Body)
	}
	return parseJSON(req.Body)
}

// isForm reports whether the request says its body is form-encoded.
func isForm(h http.Header) bool {
	mediaType, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	return err == nil && mediaType == formContentType
}

// parseForm reads a form-encoded body as an object of each parameter's value, or the list of
// its values when the name repeats.
func parseForm(raw []byte) (any, error) {
	values, err := url.ParseQuery(string(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: body is not form-encoded: %w", ErrMalformed, err)
	}
	out := make(map[string]any, len(values))
	for name, vs := range values {
		if len(vs) == 1 {
			out[name] = vs[0]
			continue
		}
		list := make([]any, len(vs))
		for i, v := range vs {
			list[i] = v
		}
		out[name] = list
	}
	return out, nil
}

// parseJSON reads the body as JSON. Integers survive exactly.
func parseJSON(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("%w: body is not JSON: %w", ErrMalformed, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: body has data after the JSON value", ErrMalformed)
	}
	return v, nil
}

// headerMap is the request headers as the transform sees them: lower-cased names,
// the first value of each.
func headerMap(h http.Header) map[string]any {
	out := make(map[string]any, len(h))
	for name, values := range h {
		if len(values) > 0 {
			out[strings.ToLower(name)] = values[0]
		}
	}
	return out
}

// jsonValue round-trips a Go value to the shape jsonschema validates.
func jsonValue(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(raw))
}
