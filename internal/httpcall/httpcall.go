// Package httpcall is the outbound HTTP call both an Automation's HTTP step (ADR 0039) and a
// Rule's HTTP Action (ADR 0054) describe: a URL, request headers, a Liquid JSON body and an
// optional signing secret. It validates a call as written, seals its URL, header values and
// secret per field for storage, opens them for the executor and renders the body. It holds no
// client and no Workspace: the owner of the call decides when to make it (webhook.Post).
package httpcall

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// Limits of a call (ADR 0039): they keep one call from exhausting the queue.
const (
	MaxURLBytes  = 2 << 10
	MaxBodyBytes = 64 << 10
	MaxHeaders   = 5
)

// ErrInvalidBody: the body did not render to valid JSON within the size limit.
var ErrInvalidBody = errors.New("automations: body does not render to valid JSON")

// InvalidError is a call that does not fit the rules of a call. Its text is safe to show to the author.
type InvalidError string

func (e InvalidError) Error() string { return string(e) }

// Header is one request header. In a stored call Value is ciphertext. In a saved call, an empty
// Value keeps the stored value of the header with the same name.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// Spec is a call. Stored, URL, header values and Secret are ciphertext; in a call being saved,
// an empty one keeps the stored value, and ClearSecret drops the signing secret.
type Spec struct {
	URL         string
	Headers     []Header
	Body        string
	Secret      string
	ClearSecret bool
}

// Opened is a call's secrets in the clear, for the executor.
type Opened struct {
	URL     string
	Headers map[string]string
	Secret  string
}

var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// reservedHeaders are set by sphericon itself and cannot be overridden by a call.
var reservedHeaders = map[string]bool{
	"content-type": true, "content-length": true, "host": true,
	"webhook-id": true, "webhook-timestamp": true, "webhook-signature": true,
}

// RenderBody renders a Liquid JSON body against bindings and checks the result is valid JSON
// within the size limit. The bindings' strings are JSON-escaped, so a value holding quotes
// cannot change the structure.
func RenderBody(body string, bindings map[string]any) (string, error) {
	out, err := emailrender.Render(body, escapeBindings(bindings))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidBody, err)
	}
	if len(out) > MaxBodyBytes {
		return "", fmt.Errorf("%w: over %d bytes", ErrInvalidBody, MaxBodyBytes)
	}
	if !json.Valid([]byte(out)) {
		return "", fmt.Errorf("%w", ErrInvalidBody)
	}
	return out, nil
}

// BodyDetail is the reason of a RenderBody error, without its prefix.
func BodyDetail(err error) string {
	return strings.TrimPrefix(err.Error(), ErrInvalidBody.Error()+": ")
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

// Open decrypts a stored call's URL, header values and secret.
func Open(c *secrets.Cipher, s Spec) (Opened, error) {
	var out Opened
	u, err := open(c, s.URL)
	if err != nil {
		return out, err
	}
	out.URL = u
	out.Headers = make(map[string]string, len(s.Headers))
	for _, h := range s.Headers {
		if out.Headers[h.Name], err = open(c, h.Value); err != nil {
			return out, err
		}
	}
	out.Secret, err = open(c, s.Secret)
	return out, err
}

func open(c *secrets.Cipher, sealed string) (string, error) {
	if sealed == "" {
		return "", nil
	}
	b, err := c.Decrypt(sealed)
	return string(b), err
}

func seal(c *secrets.Cipher, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	return c.Encrypt([]byte(plain))
}

// Seal validates a call being saved and returns its stored form: URL, header values and secret
// encrypted per field. What the author did not resend (an empty URL, header value or secret) is
// copied, as stored ciphertext, from prev, the stored call it replaces (the zero Spec for a new
// one). what names the call in the errors ("http step", "http action"); a refusal is an InvalidError.
func Seal(c *secrets.Cipher, in, prev Spec, what string) (Spec, error) {
	invalid := func(format string, a ...any) error { return InvalidError(what + " " + fmt.Sprintf(format, a...)) }
	out := Spec{Body: in.Body, URL: prev.URL, Secret: prev.Secret}
	var err error
	if in.URL != "" {
		if len(in.URL) > MaxURLBytes {
			return out, invalid("url is over %d bytes", MaxURLBytes)
		}
		if u, perr := url.Parse(in.URL); perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return out, invalid("url must be an http(s) url")
		}
		if out.URL, err = seal(c, in.URL); err != nil {
			return out, err
		}
	}
	if out.URL == "" {
		return out, invalid("needs a url")
	}
	if len(in.Body) > MaxBodyBytes {
		return out, invalid("body is over %d bytes", MaxBodyBytes)
	}
	if _, err := RenderBody(in.Body, nil); err != nil {
		return out, invalid("body: %s", BodyDetail(err))
	}
	if len(in.Headers) > MaxHeaders {
		return out, invalid("has at most %d headers", MaxHeaders)
	}
	kept := map[string]string{}
	for _, h := range prev.Headers {
		kept[strings.ToLower(h.Name)] = h.Value
	}
	names := map[string]bool{}
	for _, h := range in.Headers {
		key := strings.ToLower(h.Name)
		if !headerName.MatchString(h.Name) || reservedHeaders[key] || names[key] {
			return out, invalid("header %q is not allowed", h.Name)
		}
		names[key] = true
		value := kept[key]
		if h.Value != "" {
			if value, err = seal(c, h.Value); err != nil {
				return out, err
			}
		}
		if value == "" {
			return out, invalid("header %q needs a value", h.Name)
		}
		out.Headers = append(out.Headers, Header{Name: h.Name, Value: value})
	}
	switch {
	case in.ClearSecret:
		out.Secret = ""
	case in.Secret != "":
		if out.Secret, err = seal(c, in.Secret); err != nil {
			return out, err
		}
	}
	return out, nil
}
