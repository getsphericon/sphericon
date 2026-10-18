package preset

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
)

// verifyConfig is the `verify` block of a Preset file: one kind from a closed set
// implemented here, configured by the Preset. The file schema holds which fields each
// kind requires.
type verifyConfig struct {
	Kind            string   `json:"kind"`
	Secret          string   `json:"secret"`
	Header          string   `json:"header"`
	Prefix          string   `json:"prefix"`
	Signed          string   `json:"signed"`
	Algorithm       string   `json:"algorithm"`
	Encoding        string   `json:"encoding"`
	TimestampHeader string   `json:"timestamp_header"`
	TimestampField  string   `json:"timestamp_field"`
	Tolerance       string   `json:"tolerance"`
	Basic           bool     `json:"basic"`
	User            string   `json:"user"`
	SignedFields    []string `json:"signed_fields"`
	SignatureField  string   `json:"signature_field"`
}

// verifier checks one request. overBody is true for a scheme whose signature lives in
// the body: the engine parses the body under its limit first and hands it over.
type verifier interface {
	overBody() bool
	verify(req Request, body any, params Params, now time.Time) error
}

func newVerifier(c verifyConfig) (verifier, error) {
	var tolerance time.Duration
	if c.Tolerance != "" {
		d, err := time.ParseDuration(c.Tolerance)
		if err != nil {
			return nil, fmt.Errorf("verify: tolerance: %w", err)
		}
		tolerance = d
	}
	switch c.Kind {
	case "hmac_header":
		tmpl, err := parseTemplate(c.Signed)
		if err != nil {
			return nil, err
		}
		return &hmacHeader{c: c, tmpl: tmpl, tolerance: tolerance}, nil
	case "standard_webhooks":
		return &standardWebhooksVerifier{secret: c.Secret}, nil
	case "token":
		return &tokenVerifier{c: c}, nil
	case "hmac_body":
		return &hmacBody{c: c, tolerance: tolerance}, nil
	}
	return nil, fmt.Errorf("verify: unknown kind %q", c.Kind)
}

var errNoSecret = errors.New("the Integration has no secret for this Preset")

// secretsOf reads a secret param: a string, or a list (current first, retiring after).
func secretsOf(params Params, name string) []string {
	switch v := params[name].(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []string:
		return slices.DeleteFunc(slices.Clone(v), func(s string) bool { return s == "" })
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func hasherFor(algorithm string) func() hash.Hash {
	switch algorithm {
	case "sha1":
		return sha1.New
	case "sha512":
		return sha512.New
	default:
		return sha256.New
	}
}

func decodeSignature(encoding, s string) ([]byte, error) {
	if encoding == "base64" {
		return base64.StdEncoding.DecodeString(s)
	}
	return hex.DecodeString(s)
}

// macMatches reports whether signature is the HMAC of content under any secret, in
// constant time.
func macMatches(newHash func() hash.Hash, secrets []string, content, signature []byte) bool {
	ok := false
	for _, secret := range secrets {
		m := hmac.New(newHash, []byte(secret))
		m.Write(content)
		if hmac.Equal(m.Sum(nil), signature) {
			ok = true
		}
	}
	return ok
}

// withinTolerance checks a unix-seconds timestamp against the clock, both ways.
func withinTolerance(raw string, tolerance time.Duration, now time.Time) error {
	secs, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return fmt.Errorf("timestamp is not a unix time: %w", err)
	}
	if now.Sub(time.Unix(secs, 0)).Abs() > tolerance {
		return errors.New("timestamp is outside the tolerance")
	}
	return nil
}

// hmac_header ---------------------------------------------------------------------

var placeholder = regexp.MustCompile(`\{(body|header\.[A-Za-z0-9-]+)\}`)

type templatePart struct {
	literal string
	ref     string // "body" or "header.<name>"; empty for a literal
}

func parseTemplate(t string) ([]templatePart, error) {
	var parts []templatePart
	last := 0
	for _, m := range placeholder.FindAllStringSubmatchIndex(t, -1) {
		if m[0] > last {
			parts = append(parts, templatePart{literal: t[last:m[0]]})
		}
		parts = append(parts, templatePart{ref: t[m[2]:m[3]]})
		last = m[1]
	}
	if last < len(t) {
		parts = append(parts, templatePart{literal: t[last:]})
	}
	if !strings.Contains(t, "{body}") {
		return nil, errors.New("verify: signed must contain {body}")
	}
	return parts, nil
}

type hmacHeader struct {
	c         verifyConfig
	tmpl      []templatePart
	tolerance time.Duration
}

func (*hmacHeader) overBody() bool { return false }

func (h *hmacHeader) verify(req Request, _ any, params Params, now time.Time) error {
	secrets := secretsOf(params, h.c.Secret)
	if len(secrets) == 0 {
		return errNoSecret
	}
	value := req.Header.Get(h.c.Header)
	if value == "" || !strings.HasPrefix(value, h.c.Prefix) {
		return fmt.Errorf("header %s is missing", h.c.Header)
	}
	signature, err := decodeSignature(h.c.Encoding, strings.TrimPrefix(value, h.c.Prefix))
	if err != nil {
		return fmt.Errorf("header %s is not %s: %w", h.c.Header, h.c.Encoding, err)
	}
	if h.c.TimestampHeader != "" {
		if err := withinTolerance(req.Header.Get(h.c.TimestampHeader), h.tolerance, now); err != nil {
			return err
		}
	}
	var content []byte
	for _, p := range h.tmpl {
		switch {
		case p.ref == "body":
			content = append(content, req.Body...)
		case p.ref != "":
			content = append(content, req.Header.Get(strings.TrimPrefix(p.ref, "header."))...)
		default:
			content = append(content, p.literal...)
		}
	}
	if !macMatches(hasherFor(h.c.Algorithm), secrets, content, signature) {
		return errors.New("signature does not match")
	}
	return nil
}

// standard_webhooks ----------------------------------------------------------------

type standardWebhooksVerifier struct{ secret string }

func (*standardWebhooksVerifier) overBody() bool { return false }

// standardWebhooksTolerance is the specification's.
const standardWebhooksTolerance = 5 * time.Minute

func (s *standardWebhooksVerifier) verify(req Request, _ any, params Params, now time.Time) error {
	secrets := secretsOf(params, s.secret)
	if len(secrets) == 0 {
		return errNoSecret
	}
	// The library reads the wall clock, so it verifies the signature only and the
	// timestamp is checked here against the injected clock.
	if err := withinTolerance(req.Header.Get(standardwebhooks.HeaderWebhookTimestamp), standardWebhooksTolerance, now); err != nil {
		return err
	}
	var last error
	for _, secret := range secrets {
		wh, err := standardwebhooks.NewWebhook(secret)
		if err != nil {
			last = err
			continue
		}
		if last = wh.VerifyIgnoringTimestamp(req.Body, req.Header); last == nil {
			return nil
		}
	}
	return last
}

// token ----------------------------------------------------------------------------

type tokenVerifier struct{ c verifyConfig }

func (*tokenVerifier) overBody() bool { return false }

func (t *tokenVerifier) verify(req Request, _ any, params Params, _ time.Time) error {
	secrets := secretsOf(params, t.c.Secret)
	if len(secrets) == 0 {
		return errNoSecret
	}
	var got string
	if t.c.Basic {
		user, pass, ok := req.basicAuth()
		if !ok {
			return errors.New("basic auth is missing")
		}
		if want := secretsOf(params, t.c.User); t.c.User != "" && !anyEqual(want, user) {
			return errors.New("basic auth user does not match")
		}
		got = pass
	} else {
		value := req.Header.Get(t.c.Header)
		if value == "" || !strings.HasPrefix(value, t.c.Prefix) {
			return fmt.Errorf("header %s is missing", t.c.Header)
		}
		got = strings.TrimPrefix(value, t.c.Prefix)
	}
	if !anyEqual(secrets, got) {
		return errors.New("token does not match")
	}
	return nil
}

func (r Request) basicAuth() (user, pass string, ok bool) {
	return (&http.Request{Header: r.Header}).BasicAuth()
}

// anyEqual compares in constant time against every candidate.
func anyEqual(candidates []string, got string) bool {
	ok := 0
	for _, c := range candidates {
		ok |= subtle.ConstantTimeCompare([]byte(c), []byte(got))
	}
	return ok == 1
}

// hmac_body ------------------------------------------------------------------------

type hmacBody struct {
	c         verifyConfig
	tolerance time.Duration
}

func (*hmacBody) overBody() bool { return true }

func (h *hmacBody) verify(_ Request, body any, params Params, now time.Time) error {
	secrets := secretsOf(params, h.c.Secret)
	if len(secrets) == 0 {
		return errNoSecret
	}
	sigText, err := bodyField(body, h.c.SignatureField)
	if err != nil {
		return err
	}
	signature, err := decodeSignature(h.c.Encoding, sigText)
	if err != nil {
		return fmt.Errorf("%s is not %s: %w", h.c.SignatureField, h.c.Encoding, err)
	}
	if h.c.TimestampField != "" {
		ts, err := bodyField(body, h.c.TimestampField)
		if err != nil {
			return err
		}
		if err := withinTolerance(ts, h.tolerance, now); err != nil {
			return err
		}
	}
	var content []byte
	for _, path := range h.c.SignedFields {
		v, err := bodyField(body, path)
		if err != nil {
			return err
		}
		content = append(content, v...)
	}
	if !macMatches(hasherFor(h.c.Algorithm), secrets, content, signature) {
		return errors.New("signature does not match")
	}
	return nil
}

// bodyField reads a string or number at a dotted path of the parsed body.
func bodyField(body any, path string) (string, error) {
	cur := body
	for _, key := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return "", fmt.Errorf("body field %s is missing", path)
		}
		if cur, ok = m[key]; !ok {
			return "", fmt.Errorf("body field %s is missing", path)
		}
	}
	switch v := cur.(type) {
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	}
	return "", fmt.Errorf("body field %s is not a string", path)
}
