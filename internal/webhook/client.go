// Package webhook delivers domain events to customer-configured HTTP endpoints.
// It composes maintained libraries rather than hand-rolling the security-critical
// parts: doyensec/safeurl for the SSRF-hardened HTTP client and the
// standard-webhooks library for interoperable HMAC signatures.
package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/doyensec/safeurl"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
)

// maxResponseBytes caps how much of a receiver's response body we read.
const maxResponseBytes = 64 << 10

// ExcerptBytes is how much of a receiver's answer a failed call keeps.
const ExcerptBytes = 1 << 10

// EventHeader carries the event name for receiver-side routing; the signed
// payload also carries "type". Identity/timestamp/signature use the Standard
// Webhooks headers (webhook-id, webhook-timestamp, webhook-signature).
const EventHeader = "X-sphericon-Event"

// Doer is the subset of *http.Client used for delivery; both *http.Client and
// safeurl's wrapped client satisfy it (the latter is not an *http.Client).
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewClient builds an SSRF-hardened HTTP client via doyensec/safeurl: it blocks
// dialing private/loopback/link-local/CGNAT/metadata addresses on the *resolved*
// IP (defeating DNS rebinding), restricts schemes to http(s), and does not follow
// redirects.
func NewClient(timeout time.Duration) Doer {
	cfg := safeurl.GetConfigBuilder().
		SetTimeout(timeout).
		SetAllowedSchemes("http", "https").
		EnableIPv6(true).
		SetCheckRedirect(func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // surface the 3xx; never follow it
		}).
		Build()
	return safeurl.Client(cfg)
}

// Send signs the body with the endpoint secret (Standard Webhooks scheme) and
// POSTs it via client. It errors on transport failure or a non-2xx response so
// the caller can retry. Use a client from NewClient so the SSRF guard applies.
func Send(ctx context.Context, client Doer, url, secret, eventName, deliveryID string, body []byte) error {
	return Post(ctx, client, Call{
		URL: url, Secret: secret, ID: deliveryID, Body: body,
		Headers: map[string]string{EventHeader: eventName},
	})
}

// Call is one outbound POST of a JSON body. A Secret signs it (Standard Webhooks, with ID
// as the message id); an empty Secret sends it unsigned. Headers are extra request
// headers, set after the standard ones.
type Call struct {
	URL     string
	Secret  string
	ID      string
	Headers map[string]string
	Body    []byte
}

// Post makes the call through client, which should come from NewClient or
// NewClientAllowing. It returns a *DeliveryError for a transport failure or a non-2xx
// answer; the latter carries the first ExcerptBytes of the receiver's response.
func Post(ctx context.Context, client Doer, c Call) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(c.Body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.Secret != "" {
		wh, err := standardwebhooks.NewWebhook(c.Secret)
		if err != nil {
			return fmt.Errorf("webhook secret: %w", err)
		}
		ts := time.Now()
		sig, err := wh.Sign(c.ID, ts, c.Body)
		if err != nil {
			return fmt.Errorf("sign webhook: %w", err)
		}
		req.Header.Set(standardwebhooks.HeaderWebhookID, c.ID)
		req.Header.Set(standardwebhooks.HeaderWebhookTimestamp, strconv.FormatInt(ts.Unix(), 10))
		req.Header.Set(standardwebhooks.HeaderWebhookSignature, sig)
	}
	for name, value := range c.Headers {
		req.Header.Set(name, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return transportError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	answer, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		failure := statusError(resp.StatusCode, resp.Header.Get("Retry-After"))
		failure.Excerpt = excerpt(answer)
		return failure
	}
	return nil
}

// excerpt is the first ExcerptBytes of an answer, with a character cut by the limit dropped.
func excerpt(answer []byte) string {
	return strings.ToValidUTF8(string(answer[:min(len(answer), ExcerptBytes)]), "")
}
