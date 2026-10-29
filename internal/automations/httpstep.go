package automations

import (
	"context"
	"errors"
	"fmt"

	"github.com/oklog/ulid/v2"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/httpcall"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/validation"
)

// Limits of an HTTP step (ADR 0039): they keep one step from exhausting the queue.
const (
	MaxHTTPURLBytes           = httpcall.MaxURLBytes
	MaxHTTPBodyBytes          = httpcall.MaxBodyBytes
	MaxHTTPHeaders            = httpcall.MaxHeaders
	MaxHTTPStepsPerAutomation = 10
)

// ErrorKindInvalidBody is the closed error kind of a body that does not render to
// valid JSON (a template error, invalid JSON or an oversized body).
const ErrorKindInvalidBody = "invalid_body"

var (
	// ErrContactNotFound: no such Contact in this Workspace.
	ErrContactNotFound = validation.Refuse("automations: contact not found", "errors.contact_not_found")
	// ErrInvalidBody: the body did not render to valid JSON within the size limit.
	ErrInvalidBody = httpcall.ErrInvalidBody
)

// Header is one request header of an HTTP step. In a stored step Value is
// ciphertext. In a saved step, an empty Value keeps the stored value of the header
// with the same name.
type Header = httpcall.Header

// OpenedHTTP is an HTTP step's secrets in the clear, for the executor.
type OpenedHTTP = httpcall.Opened

// DryRunResult is a body rendered for one Contact, or the kind of error that
// stopped it. Nothing is called.
type DryRunResult struct {
	Body      string
	ErrorKind string
	Detail    string
}

// RenderBody renders an HTTP step's Liquid body for a Contact (nil: no bindings)
// and checks the result is valid JSON within the size limit. The Contact's strings
// are JSON-escaped in the bindings themselves, so a value holding quotes cannot
// change the structure.
func RenderBody(body string, c *ent.Contact) (string, error) {
	return httpcall.RenderBody(body, outbound.Bindings(c, nil))
}

func bodyDetail(err error) string {
	return httpcall.BodyDetail(err)
}

// DryRun renders a body for a Contact of the Workspace and validates it without
// calling anything.
func (m *Module) DryRun(ctx context.Context, s *ent.Scoped, contactID int64, body string) (DryRunResult, error) {
	c, err := s.Contact().Get(ctx, contactID)
	if ent.IsNotFound(err) {
		return DryRunResult{}, ErrContactNotFound
	}
	if err != nil {
		return DryRunResult{}, err
	}
	out, err := RenderBody(body, c)
	if err != nil {
		return DryRunResult{ErrorKind: ErrorKindInvalidBody, Detail: bodyDetail(err)}, nil
	}
	return DryRunResult{Body: out}, nil
}

// OpenHTTP decrypts a stored HTTP step's URL, header values and secret with the cipher that sealed
// them.
func OpenHTTP(cipher *secrets.Cipher, s Step) (OpenedHTTP, error) {
	return httpcall.Open(cipher, spec(s))
}

func spec(s Step) httpcall.Spec {
	return httpcall.Spec{URL: s.URL, Headers: s.Headers, Body: s.Body, Secret: s.Secret, ClearSecret: s.ClearSecret}
}

func invalid(format string, a ...any) error {
	return ErrInvalidStep.With(fmt.Sprintf(format, a...))
}

// sealSteps validates the steps and turns every HTTP step into its stored form:
// URL, header values and secret encrypted per field. What the author did not resend
// (an empty URL, header value or secret) is copied, as stored ciphertext, from the
// stored step with the same id.
func (m *Module) sealSteps(steps, stored []Step) ([]Step, error) {
	prev := map[string]Step{}
	for i := range stored {
		if stored[i].Type == StepHTTP {
			prev[stored[i].ID] = stored[i]
		}
	}
	out := make([]Step, len(steps))
	seen := map[string]bool{}
	for i := range steps {
		s := steps[i]
		if s.Type != StepHTTP {
			out[i] = s
			continue
		}
		if len(seen) >= MaxHTTPStepsPerAutomation {
			return nil, invalid("an automation has at most %d http steps", MaxHTTPStepsPerAutomation)
		}
		p, known := prev[s.ID]
		sealed, err := m.sealHTTP(s, p, known && s.ID != "")
		if err != nil {
			return nil, err
		}
		if seen[sealed.ID] {
			return nil, invalid("duplicate http step id")
		}
		seen[sealed.ID] = true
		out[i] = sealed
	}
	return out, nil
}

func (m *Module) sealHTTP(s, prev Step, known bool) (Step, error) {
	out := Step{Type: StepHTTP, ID: s.ID}
	if !known {
		prev = Step{}
		out.ID = ulid.Make().String()
	}
	sealed, err := httpcall.Seal(m.cipher, spec(s), spec(prev), "http step")
	if bad, ok := errors.AsType[httpcall.InvalidError](err); ok {
		return out, invalid("%s", bad)
	}
	out.URL, out.Headers, out.Body, out.Secret = sealed.URL, sealed.Headers, sealed.Body, sealed.Secret
	return out, err
}
