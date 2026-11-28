package automations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/automationstep"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// Limits of an HTTP step (ADR 0039): they keep one step from exhausting the queue.
const (
	MaxHTTPURLBytes           = httpcall.MaxURLBytes
	MaxHTTPBodyBytes          = httpcall.MaxBodyBytes
	MaxHTTPStepsPerAutomation = 10
)

// ErrorKindInvalidBody is the closed error kind of a body that does not render to
// valid JSON (a template error, invalid JSON or an oversized body).
const ErrorKindInvalidBody = "invalid_body"

var (
	// ErrContactNotFound: no such Contact in this Workspace.
	ErrContactNotFound = validation.NotFound("automations: contact not found", i18n.ErrorsContactNotFound)
	// ErrInvalidBody: the body did not render to valid JSON within the size limit.
	ErrInvalidBody = httpcall.ErrInvalidBody
)

// DryRunResult is a body rendered for one Contact, or the kind of error that
// stopped it. Nothing is called.
type DryRunResult struct {
	Body      string
	ErrorKind string
	Detail    string
}

// renderBody renders an HTTP step's Liquid body for a Contact (nil: no Contact bindings) and an
// Enrollment run (nil: an empty `event`) that keeps what its earlier Steps received (ADR 0136, ADR
// 0161), and checks the result is valid JSON within the size limit. It binds the Contact's fields,
// `event`, the copy of the Event the Enrollment began on (#752), as a Rule's http Action binds the
// Event it fired on, and `awaited` and `http`, as a send step does. Every value the body writes is
// JSON-escaped as it is written, so a value holding quotes cannot change the structure.
func renderBody(body string, c *ent.Contact, run *ent.AutomationRun, kept outboundmessages.Kept) (string, error) {
	return httpcall.RenderBody(body, outboundmessages.Bindings(c, outboundmessages.EnrollmentVariables(run, kept)))
}

func bodyDetail(err error) string {
	return httpcall.BodyDetail(err)
}

// DryRun renders a body for a Contact of the Workspace and validates it without
// calling anything. No Enrollment stands behind it, so its `event`, `awaited` and `http` are empty.
func (m *Module) DryRun(ctx context.Context, s *ent.Scoped, contactID int64, body string) (DryRunResult, error) {
	c, err := s.Contact().Get(ctx, contactID)
	if ent.IsNotFound(err) {
		return DryRunResult{}, ErrContactNotFound
	}
	if err != nil {
		return DryRunResult{}, err
	}
	out, err := renderBody(body, c, nil, outboundmessages.Kept{})
	if err != nil {
		return DryRunResult{ErrorKind: ErrorKindInvalidBody, Detail: bodyDetail(err)}, nil
	}
	return DryRunResult{Body: out}, nil
}

// openHTTP decrypts a stored HTTP step's URL, header values and secret with the cipher that sealed
// them.
func openHTTP(cipher *secrets.Cipher, s automationstep.HTTP) (httpcall.Opened, error) {
	return httpcall.Open(cipher, spec(s))
}

func spec(s automationstep.HTTP) httpcall.Spec {
	return httpcall.Spec{URL: s.URL, Headers: s.Headers, Body: s.Body, Secret: s.Secret, ClearSecret: s.ClearSecret}
}

// sealSteps validates the steps, refusing one by its id and kind, and turns every HTTP step into its stored form: URL, header
// values and secret encrypted per field. What the author did not resend (an empty URL, header
// value or secret) is copied, as stored ciphertext, from the stored HTTP step with the same id
// (ADR 0122): a step whose id the stored ones do not hold is new and keeps nothing.
func (m *Module) sealSteps(steps, stored automationstep.Path) (automationstep.Path, error) {
	prev := map[automationstep.ID]automationstep.HTTP{}
	for _, st := range automationstep.All(stored) {
		if h, ok := st.Payload.(automationstep.HTTP); ok {
			prev[st.ID] = h
		}
	}
	calls := 0
	return automationstep.Map(steps, func(_ int, st *automationstep.Step) (automationstep.Step, error) {
		h, ok := st.Payload.(automationstep.HTTP)
		if !ok {
			return *st, nil
		}
		if calls >= MaxHTTPStepsPerAutomation {
			return automationstep.Step{}, refuse(st, fmt.Sprintf("an automation has at most %d http steps", MaxHTTPStepsPerAutomation))
		}
		calls++
		sealed, err := m.sealHTTP(h, prev[st.ID])
		if bad, ok := errors.AsType[httpcall.InvalidError](err); ok {
			return automationstep.Step{}, refuse(st, bad.Error())
		}
		return automationstep.Step{ID: st.ID, Payload: sealed}, err
	})
}

func (m *Module) sealHTTP(s, prev automationstep.HTTP) (automationstep.HTTP, error) {
	sealed, err := httpcall.Seal(m.cipher, spec(s), spec(prev), "http step")
	if err != nil {
		return automationstep.HTTP{}, err
	}
	return automationstep.HTTP{Name: s.Name, URL: sealed.URL, Headers: sealed.Headers, Body: sealed.Body, Secret: sealed.Secret}, err
}

// keptAnswer is what an HTTP step keeps of the 2xx answer a (ADR 0161): its status and, when the
// body is a JSON object it can keep, the body, else why it dropped it.
func keptAnswer(a webhook.Answer) *responseKept {
	status := narrow.Int32(a.Status)
	kept := &responseKept{status: &status}
	body, dropped := keptBody(a)
	if dropped != "" {
		kept.dropped = &dropped
		return kept
	}
	kept.body = body
	return kept
}

// keptBody is the body of the answer a as the JSON object an HTTP step keeps, or why it keeps none:
// one cut by the read limit, or weighing more than an Event's properties may
// (events.BoundedProperties), is too large; one that is no JSON document, or holds a NUL character
// PostgreSQL cannot store, is not JSON; and a JSON document other than an object is no object.
func keptBody(a webhook.Answer) (map[string]any, automationstep.BodyDropped) {
	if a.Cut {
		return nil, automationstep.BodyTooLarge
	}
	var doc any
	if err := json.Unmarshal(a.Body, &doc); err != nil {
		return nil, automationstep.BodyNotJSON
	}
	body, ok := doc.(map[string]any)
	if !ok {
		return nil, automationstep.BodyNotObject
	}
	bounded := events.BoundedProperties(body)
	if len(body) > 0 && bounded == nil {
		return nil, automationstep.BodyTooLarge
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, automationstep.BodyNotJSON
	}
	holdsNUL := bytes.Contains(encoded, []byte(`\u0000`))
	if holdsNUL {
		return nil, automationstep.BodyNotJSON
	}
	return body, ""
}
