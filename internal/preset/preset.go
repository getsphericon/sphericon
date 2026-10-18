// Package preset is the Preset engine (ADR 0037): a Preset describes one provider's
// inbound callbacks, how a request is verified and how its payload becomes Delivery
// failures. The engine does no I/O: it takes a Preset, the raw request and the
// Integration's params and returns the validated Delivery failures, an optional
// action for the caller to perform, or a typed error.
package preset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Limits on what one request may make the engine do.
const (
	// MaxBodyBytes is the largest request body the engine parses.
	MaxBodyBytes = 1 << 20
	// MaxProgramBytes is the largest transform program.
	MaxProgramBytes = 16 << 10
	// MaxFailures is the most Delivery failures one request may yield.
	MaxFailures = 100
	// TransformTimeout bounds one run of a transform.
	TransformTimeout = 100 * time.Millisecond
)

// The three ways a request is refused. A caller answers 4xx for all of them: the
// request is the provider's to fix, a retry would not change it.
var (
	// ErrUnauthentic: the request did not come from the provider for this Integration.
	ErrUnauthentic = errors.New("preset: request is not authentic")
	// ErrMalformed: the body is not JSON, or is over the limit.
	ErrMalformed = errors.New("preset: request is malformed")
	// ErrTransformFailed: the transform failed, timed out or returned something that is
	// not a list of valid Delivery failures.
	ErrTransformFailed = errors.New("preset: transform failed")
)

// Kind of a Delivery failure.
type Kind string

const (
	Bounce    Kind = "bounce"
	Complaint Kind = "complaint"
)

// DeliveryFailure is the one normalized shape every Preset produces. Only a permanent
// bounce or a complaint enters Suppression; a transient bounce still counts in the
// deliverability rates.
type DeliveryFailure struct {
	// ID is the provider's event id; with Destination it dedupes a redelivery.
	ID string `json:"id"`
	// Destination is the normalized (trimmed, lower-cased) address.
	Destination string `json:"destination"`
	Kind        Kind   `json:"kind"`
	// Permanent is meaningful for a bounce only.
	Permanent bool `json:"permanent"`
	// SendingDomain is the lower-cased domain the message was sent from, "" when unknown.
	SendingDomain string `json:"sending_domain,omitempty"`
}

// ActionKind is something the caller performs for the engine.
type ActionKind string

// ActionConfirm: fetch URL to confirm a subscription (SNS).
const ActionConfirm ActionKind = "confirm"

// Action is a side effect the engine asks of its caller, which performs it through the
// SSRF-guarded client.
type Action struct {
	Kind ActionKind
	URL  string
}

// Result of a verified request.
type Result struct {
	Failures []DeliveryFailure
	Action   *Action
}

// Params are the values the Owner entered on the Integration, validated against the
// Preset's params schema. A secret is a string, or a list of strings (the current one
// first, then the one being retired) while it rotates.
type Params map[string]any

// Request is what arrived at the hook.
type Request struct {
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

// Handle verifies the request and turns its payload into Delivery failures. Nothing
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
		if body, err = parseBody(req.Body); err != nil {
			return Result{}, err
		}
		if err := p.verifier.verify(req, body, params, now); err != nil {
			return Result{}, fmt.Errorf("%w: %w", ErrUnauthentic, err)
		}
	} else {
		if err := p.verifier.verify(req, nil, params, now); err != nil {
			return Result{}, fmt.Errorf("%w: %w", ErrUnauthentic, err)
		}
		var err error
		if body, err = parseBody(req.Body); err != nil {
			return Result{}, err
		}
	}
	failures, err := p.run(ctx, body, req.Header)
	if err != nil {
		return Result{}, err
	}
	return Result{Failures: failures}, nil
}

// parseBody reads the body as JSON whatever its Content-Type. Integers survive exactly.
func parseBody(raw []byte) (any, error) {
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
