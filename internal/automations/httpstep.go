package automations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/oklog/ulid/v2"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/outbound"
)

// Limits of an HTTP step (ADR 0039): they keep one step from exhausting the queue.
const (
	MaxHTTPURLBytes           = 2 << 10
	MaxHTTPBodyBytes          = 64 << 10
	MaxHTTPHeaders            = 5
	MaxHTTPStepsPerAutomation = 10
)

// ErrorKindInvalidBody is the closed error kind of a body that does not render to
// valid JSON (a template error, invalid JSON or an oversized body).
const ErrorKindInvalidBody = "invalid_body"

var (
	// ErrContactNotFound: no such Contact in this Workspace.
	ErrContactNotFound = errors.New("automations: contact not found")
	// ErrInvalidBody: the body did not render to valid JSON within the size limit.
	ErrInvalidBody = errors.New("automations: body does not render to valid JSON")
)

// Header is one request header of an HTTP step. In a stored step Value is
// ciphertext. In a saved step, an empty Value keeps the stored value of the header
// with the same name.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// OpenedHTTP is an HTTP step's secrets in the clear, for the executor.
type OpenedHTTP struct {
	URL     string
	Headers map[string]string
	Secret  string
}

// DryRunResult is a body rendered for one Contact, or the kind of error that
// stopped it. Nothing is called.
type DryRunResult struct {
	Body      string
	ErrorKind string
	Detail    string
}

var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// reservedHeaders are set by sphericon itself and cannot be overridden by a step.
var reservedHeaders = map[string]bool{
	"content-type": true, "content-length": true, "host": true,
	"webhook-id": true, "webhook-timestamp": true, "webhook-signature": true,
}

// RenderBody renders an HTTP step's Liquid body for a Contact (nil: no bindings)
// and checks the result is valid JSON within the size limit. The Contact's strings
// are JSON-escaped in the bindings themselves, so a value holding quotes cannot
// change the structure.
func RenderBody(body string, c *ent.Contact) (string, error) {
	out, err := emailrender.Render(body, escapeBindings(outbound.Bindings(c, nil)))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidBody, err)
	}
	if len(out) > MaxHTTPBodyBytes {
		return "", fmt.Errorf("%w: over %d bytes", ErrInvalidBody, MaxHTTPBodyBytes)
	}
	if !json.Valid([]byte(out)) {
		return "", fmt.Errorf("%w", ErrInvalidBody)
	}
	return out, nil
}

func escapeBindings(b map[string]any) map[string]any {
	out := make(map[string]any, len(b))
	for k, v := range b {
		out[k] = escapeValue(v)
	}
	return out
}

func escapeValue(v any) any {
	switch t := v.(type) {
	case string:
		quoted, _ := json.Marshal(t)
		return string(quoted[1 : len(quoted)-1])
	case map[string]any:
		return escapeBindings(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = escapeValue(e)
		}
		return out
	default:
		return v
	}
}

func bodyDetail(err error) string {
	return strings.TrimPrefix(err.Error(), ErrInvalidBody.Error()+": ")
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

// OpenHTTP decrypts a stored HTTP step's URL, header values and secret.
func (m *Module) OpenHTTP(s Step) (OpenedHTTP, error) {
	var out OpenedHTTP
	u, err := m.open(s.URL)
	if err != nil {
		return out, err
	}
	out.URL = u
	out.Headers = make(map[string]string, len(s.Headers))
	for _, h := range s.Headers {
		if out.Headers[h.Name], err = m.open(h.Value); err != nil {
			return out, err
		}
	}
	out.Secret, err = m.open(s.Secret)
	return out, err
}

func (m *Module) open(sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	b, err := m.cipher.Decrypt(sealed)
	return string(b), err
}

func (m *Module) seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return m.cipher.Encrypt([]byte(plain))
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidStep}, a...)...)
}

// sealSteps validates the steps and turns every HTTP step into its stored form:
// URL, header values and secret encrypted per field. What the author did not resend
// (an empty URL, header value or secret) is copied, as stored ciphertext, from the
// stored step with the same id.
func (m *Module) sealSteps(steps, stored []Step) ([]Step, error) {
	prev := map[string]Step{}
	for _, p := range stored {
		if p.Type == StepHTTP {
			prev[p.ID] = p
		}
	}
	out := make([]Step, len(steps))
	seen := map[string]bool{}
	for i, s := range steps {
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
	out := Step{Type: StepHTTP, ID: s.ID, Body: s.Body, URL: prev.URL, Secret: prev.Secret}
	if !known {
		prev = Step{}
		out.ID, out.URL, out.Secret = ulid.Make().String(), "", ""
	}
	var err error
	if s.URL != "" {
		if len(s.URL) > MaxHTTPURLBytes {
			return out, invalid("http step url is over %d bytes", MaxHTTPURLBytes)
		}
		if u, perr := url.Parse(s.URL); perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return out, invalid("http step url must be an http(s) url")
		}
		if out.URL, err = m.seal(s.URL); err != nil {
			return out, err
		}
	}
	if out.URL == "" {
		return out, invalid("http step needs a url")
	}
	if len(s.Body) > MaxHTTPBodyBytes {
		return out, invalid("http step body is over %d bytes", MaxHTTPBodyBytes)
	}
	if _, err := RenderBody(s.Body, nil); err != nil {
		return out, invalid("http step body: %s", bodyDetail(err))
	}
	if len(s.Headers) > MaxHTTPHeaders {
		return out, invalid("http step has at most %d headers", MaxHTTPHeaders)
	}
	kept := map[string]string{}
	for _, h := range prev.Headers {
		kept[strings.ToLower(h.Name)] = h.Value
	}
	names := map[string]bool{}
	for _, h := range s.Headers {
		key := strings.ToLower(h.Name)
		if !headerName.MatchString(h.Name) || reservedHeaders[key] || names[key] {
			return out, invalid("http step header %q is not allowed", h.Name)
		}
		names[key] = true
		value := kept[key]
		if h.Value != "" {
			if value, err = m.seal(h.Value); err != nil {
				return out, err
			}
		}
		if value == "" {
			return out, invalid("http step header %q needs a value", h.Name)
		}
		out.Headers = append(out.Headers, Header{Name: h.Name, Value: value})
	}
	switch {
	case s.ClearSecret:
		out.Secret = ""
	case s.Secret != "":
		if out.Secret, err = m.seal(s.Secret); err != nil {
			return out, err
		}
	}
	return out, nil
}
