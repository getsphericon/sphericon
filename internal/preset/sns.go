package preset

import (
	"bytes"
	"context"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"github.com/getsphericon/sphericon/internal/webhook"
)

// The `sns` verify kind: the signature of an Amazon SNS message (both signature versions),
// the host the signing certificate is fetched from, and the one topic the Integration
// names. A valid signature proves AWS sent the message, not whose topic it came from, so
// the topic is what binds a hook to its Integration: anyone holding the hook URL could
// otherwise post a bounce through a topic of their own.

// snsHost is the only host a signing certificate or a SubscribeURL may live on.
var snsHost = regexp.MustCompile(`^sns\.[a-zA-Z0-9-]{3,}\.amazonaws\.com(\.cn)?$`)

// CertFetcher returns the signing certificate at an already host-checked URL.
type CertFetcher func(ctx context.Context, certURL string) (*x509.Certificate, error)

type options struct{ certs CertFetcher }

// Option configures how Presets are built.
type Option func(*options)

// WithSNSCertificates replaces how an `sns` Preset fetches the signing certificate, which
// is an HTTPS request through the SSRF-guarded client by default.
func WithSNSCertificates(f CertFetcher) Option { return func(o *options) { o.certs = f } }

func buildOptions(opts []Option) options {
	o := options{certs: fetchSNSCertificate}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// testSNSCertPEM is a certificate made for tests only: its private key was discarded after
// it signed the sample notifications in the built-in `ses` Preset's tests. A Preset's own
// tests run against it (see TestSNSCertificates), so they verify offline.
//
//go:embed snstest.pem
var testSNSCertPEM []byte

// TestSNSCertificates serves the test certificate for any URL. A Preset runs its `tests`
// with it, never in production.
func TestSNSCertificates(context.Context, string) (*x509.Certificate, error) {
	return parseCertificate(testSNSCertPEM)
}

func parseCertificate(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("the certificate is not PEM")
	}
	return x509.ParseCertificate(block.Bytes)
}

// certCache keeps the certificates fetched so far, so a flood of notifications costs one
// fetch per certificate, not one each. It is bounded: the URL is checked against the SNS
// host pattern, but a caller still picks which of its many hosts to name.
var certCache = &boundedCerts{max: 16, ttl: 6 * time.Hour, entries: map[string]cachedCert{}}

type cachedCert struct {
	cert    *x509.Certificate
	fetched time.Time
}

type boundedCerts struct {
	mu      sync.Mutex
	max     int
	ttl     time.Duration
	entries map[string]cachedCert
}

func (c *boundedCerts) get(url string, now time.Time) (*x509.Certificate, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[url]
	if !ok || now.Sub(e.fetched) > c.ttl {
		return nil, false
	}
	return e.cert, true
}

func (c *boundedCerts) put(url string, cert *x509.Certificate, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[url]; !ok && len(c.entries) >= c.max {
		for k := range c.entries { // evict any one: the cache holds a few entries
			delete(c.entries, k)
			break
		}
	}
	c.entries[url] = cachedCert{cert: cert, fetched: now}
}

func fetchSNSCertificate(ctx context.Context, certURL string) (*x509.Certificate, error) {
	if cert, ok := certCache.get(certURL, time.Now()); ok {
		return cert, nil
	}
	cert, err := downloadSNSCertificate(ctx, certURL)
	if err != nil {
		return nil, err
	}
	certCache.put(certURL, cert, time.Now())
	return cert, nil
}

func downloadSNSCertificate(ctx context.Context, certURL string) (*x509.Certificate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := webhook.NewClient(10 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("signing certificate: status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	return parseCertificate(data)
}

// awsURL accepts an https URL on an SNS host.
func awsURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" || !snsHost.MatchString(u.Host) {
		return nil, fmt.Errorf("%q is not an https URL on an SNS host", raw)
	}
	return u, nil
}

type snsVerifier struct {
	c     verifyConfig
	certs CertFetcher
}

func (*snsVerifier) overBody() bool { return true }

// snsMessage is the JSON body SNS posts to an HTTP endpoint.
type snsMessage struct {
	Message          string `json:"Message"`
	MessageID        string `json:"MessageId"`
	Signature        string `json:"Signature"`
	SignatureVersion string `json:"SignatureVersion"`
	SigningCertURL   string `json:"SigningCertURL"`
	SubscribeURL     string `json:"SubscribeURL"`
	Subject          string `json:"Subject"`
	Timestamp        string `json:"Timestamp"`
	Token            string `json:"Token"`
	TopicArn         string `json:"TopicArn"`
	Type             string `json:"Type"`
}

// stringToSign is what AWS signs: the fields its message type carries, each as its name and its
// value on lines of their own, in alphabetical order (a Notification's Subject only when it has
// one). https://docs.aws.amazon.com/sns/latest/dg/sns-verify-signature-of-message.html
func (m *snsMessage) stringToSign() ([]byte, error) {
	var fields []struct{ name, value string }
	add := func(name, value string) { fields = append(fields, struct{ name, value string }{name, value}) }
	switch m.Type {
	case "Notification":
		add("Message", m.Message)
		add("MessageId", m.MessageID)
		if m.Subject != "" {
			add("Subject", m.Subject)
		}
		add("Timestamp", m.Timestamp)
		add("TopicArn", m.TopicArn)
		add("Type", m.Type)
	case "SubscriptionConfirmation", "UnsubscribeConfirmation":
		add("Message", m.Message)
		add("MessageId", m.MessageID)
		add("SubscribeURL", m.SubscribeURL)
		add("Timestamp", m.Timestamp)
		add("Token", m.Token)
		add("TopicArn", m.TopicArn)
		add("Type", m.Type)
	default:
		return nil, fmt.Errorf("message type %q is unknown", m.Type)
	}
	var b bytes.Buffer
	for _, f := range fields {
		b.WriteString(f.name + "\n" + f.value + "\n")
	}
	return b.Bytes(), nil
}

// algorithm is the signature algorithm of the message's signature version: 1 is SHA-1, 2 is
// SHA-256, both over RSA.
func (m *snsMessage) algorithm() x509.SignatureAlgorithm {
	if m.SignatureVersion == "2" {
		return x509.SHA256WithRSA
	}
	return x509.SHA1WithRSA
}

func snsPayload(body any) (*snsMessage, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var p snsMessage
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("the body is not an SNS message: %w", err)
	}
	return &p, nil
}

func (s *snsVerifier) verify(ctx context.Context, _ Request, body any, params Params, _ time.Time) error {
	p, err := snsPayload(body)
	if err != nil {
		return err
	}
	if p.SignatureVersion != "1" && p.SignatureVersion != "2" {
		return fmt.Errorf("signature version %q is unknown", p.SignatureVersion)
	}
	signature, err := base64.StdEncoding.DecodeString(p.Signature)
	if err != nil {
		return fmt.Errorf("signature is not base64: %w", err)
	}
	// The topic first: it costs nothing, and a message for another topic never makes us
	// fetch a certificate.
	topic, _ := params[s.c.Topic].(string)
	if topic == "" {
		return errors.New("the Integration names no SNS topic")
	}
	if p.TopicArn != topic {
		return errors.New("the message is from another SNS topic")
	}
	if _, err := awsURL(p.SigningCertURL); err != nil {
		return fmt.Errorf("signing certificate: %w", err)
	}
	// A certificate we cannot get says nothing about the request: ours to retry.
	cert, err := s.certs(ctx, p.SigningCertURL)
	if err != nil {
		return fmt.Errorf("%w: signing certificate: %w", ErrUnavailable, err)
	}
	signed, err := p.stringToSign()
	if err != nil {
		return err
	}
	if err := cert.CheckSignature(p.algorithm(), signed, signature); err != nil {
		return fmt.Errorf("signature does not match: %w", err)
	}
	if p.Type == "SubscriptionConfirmation" {
		if _, err := awsURL(p.SubscribeURL); err != nil {
			return fmt.Errorf("SubscribeURL: %w", err)
		}
	}
	return nil
}

// action asks the caller to confirm a subscription: the body was verified already.
func (*snsVerifier) action(body any) *Action {
	p, err := snsPayload(body)
	if err != nil || p.Type != "SubscriptionConfirmation" {
		return nil
	}
	return &Action{Kind: ActionConfirm, URL: p.SubscribeURL}
}
