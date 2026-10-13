package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/webhook"
	sns "github.com/robbiet480/go.sns"
)

// hooksHandler serves inbound provider webhooks, routed by the workspace's secret
// ingest key: POST /hooks/{key}/{provider}. Each provider has its own adapter
// that verifies the provider's signature, normalizes the payload into our typed
// domain events, and publishes them (suppression + persist subscribe downstream).
// Today the only adapter is AWS SES delivered over SNS.
func hooksHandler(client *ent.Client, bus *events.Bus) http.Handler {
	h := &sesHook{
		ent: client,
		bus: bus,
		// VerifyPayload checks the signature against the (amazonaws-hosted) signing
		// cert; injectable so tests can drive the parser without real AWS signatures.
		verify:  func(p *sns.Payload) error { return p.VerifyPayload() },
		confirm: confirmSNSSubscription,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /hooks/{key}/ses", h.handle)
	return mux
}

type sesHook struct {
	ent     *ent.Client
	bus     *events.Bus
	verify  func(*sns.Payload) error
	confirm func(ctx context.Context, subscribeURL string) error
}

// sesNotification is the SES event carried in the SNS Payload.Message string. SES
// uses "notificationType" for identity notifications and "eventType" for
// configuration-set event publishing; we accept either.
type sesNotification struct {
	NotificationType string `json:"notificationType"`
	EventType        string `json:"eventType"`
	Mail             struct {
		Source string `json:"source"` // the From address the message was sent as
	} `json:"mail"`
	Bounce *struct {
		BounceType        string `json:"bounceType"` // Permanent|Transient|Undetermined
		BouncedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"bouncedRecipients"`
	} `json:"bounce"`
	Complaint *struct {
		ComplainedRecipients []struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"complainedRecipients"`
	} `json:"complaint"`
}

func (h *sesHook) handle(w http.ResponseWriter, r *http.Request) {
	ws, err := h.ent.Workspace.Query().
		Where(workspace.IngestKey(r.PathValue("key"))).
		Only(r.Context())
	if ent.IsNotFound(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "workspace lookup failed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeProblem(w, http.StatusRequestEntityTooLarge, "body too large")
			return
		}
		writeProblem(w, http.StatusBadRequest, "read body")
		return
	}
	var payload sns.Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid SNS payload")
		return
	}
	if err := h.verify(&payload); err != nil {
		writeProblem(w, http.StatusForbidden, "SNS signature verification failed")
		return
	}

	switch payload.Type {
	case "SubscriptionConfirmation":
		// Confirm the subscription by fetching the (SSRF-guarded) SubscribeURL. On
		// failure return 5xx so SNS resends the confirmation.
		if err := h.confirm(r.Context(), payload.SubscribeURL); err != nil {
			logging.FromContext(r.Context()).Error("hooks/ses: confirm subscription failed", "workspace_id", ws.ID, "err", err)
			writeProblem(w, http.StatusBadGateway, "subscription confirmation failed")
			return
		}
	case "Notification":
		// The ingest key resolved the Workspace above: this is where its scope is built.
		// On failure return 5xx so SNS redelivers rather than dropping the bounce;
		// downstream persist/suppression dedupe on redelivery (DedupKey).
		if err := h.handleNotification(r.Context(), events.Ingest(h.ent.Scoped(ws.ID)), payload); err != nil {
			logging.FromContext(r.Context()).Error("hooks/ses: notification processing failed", "workspace_id", ws.ID, "err", err)
			writeProblem(w, http.StatusInternalServerError, "notification processing failed")
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// sesFailure is a normalized bounce/complaint extracted from an SES message.
type sesFailure struct {
	Action string // events.NameEmailBounced | events.NameEmailComplained
	Email  string
	Kind   string // permanent|transient for bounces; "" for complaints
	// SendingDomain is the domain of the From address (ADR 0011), "" when SES did not echo it.
	SendingDomain string
}

// parseSESNotification extracts the suppressing failures from an SES message
// (the JSON string carried in SNS Payload.Message). Delivery/Send and other
// types yield none. Pure (no I/O) so it is unit-testable without real payloads.
func parseSESNotification(message string) ([]sesFailure, error) {
	var n sesNotification
	if err := json.Unmarshal([]byte(message), &n); err != nil {
		return nil, fmt.Errorf("parse SES message: %w", err)
	}
	typ := n.NotificationType
	if typ == "" {
		typ = n.EventType
	}

	sendingDomain := sourceDomain(n.Mail.Source)
	var out []sesFailure
	switch typ {
	case "Bounce":
		if n.Bounce == nil {
			return nil, nil
		}
		kind := events.BounceKindTransient
		if n.Bounce.BounceType == "Permanent" {
			kind = events.BounceKindPermanent
		}
		for _, rcpt := range n.Bounce.BouncedRecipients {
			out = append(out, sesFailure{Action: events.NameEmailBounced, Email: rcpt.EmailAddress, Kind: kind, SendingDomain: sendingDomain})
		}
	case "Complaint":
		if n.Complaint == nil {
			return nil, nil
		}
		for _, rcpt := range n.Complaint.ComplainedRecipients {
			out = append(out, sesFailure{Action: events.NameEmailComplained, Email: rcpt.EmailAddress, SendingDomain: sendingDomain})
		}
	}
	// Other SES types (Delivery, Send, …) yield no failures.
	return out, nil
}

func (h *sesHook) handleNotification(ctx context.Context, s *ent.Scoped, payload sns.Payload) error {
	failures, err := parseSESNotification(payload.Message)
	if err != nil {
		return err
	}
	for _, f := range failures {
		if err := h.publishFailure(ctx, s, payload.MessageId, f); err != nil {
			return err
		}
	}
	return nil
}

// sourceDomain extracts the lower-cased domain of a From address ("Name <a@b.c>" or
// "a@b.c"); "" when it cannot be parsed.
func sourceDomain(source string) string {
	addr, err := mail.ParseAddress(source)
	if err != nil {
		return ""
	}
	_, domain, ok := strings.Cut(addr.Address, "@")
	if !ok {
		return ""
	}
	return strings.ToLower(domain)
}

// publishFailure normalizes the address, resolves the contact when known, and
// publishes one typed EmailDeliveryFailure keyed by SNS messageId + recipient so
// a redelivered notification dedupes downstream.
func (h *sesHook) publishFailure(ctx context.Context, s *ent.Scoped, messageID string, f sesFailure) error {
	action, kind := f.Action, f.Kind
	email := strings.ToLower(strings.TrimSpace(f.Email))
	if email == "" {
		return nil
	}
	var contactID int64
	c, err := s.Contact().Query().
		Where(contact.EmailEqualFold(email)).
		Only(ctx)
	switch {
	case err == nil:
		contactID = c.ID
	case ent.IsNotFound(err):
		// No contact for this address (e.g. a bounce for an address we never had
		// as a contact) — suppress it anyway, with no contact link.
	default:
		return fmt.Errorf("lookup contact %q: %w", email, err)
	}
	return h.bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return pub.Publish(ctx, &events.EmailDeliveryFailure{
			Action:        action,
			WorkspaceID:   s.WorkspaceID(),
			ContactID:     contactID,
			Email:         email,
			BounceKind:    kind,
			Provider:      "ses",
			SendingDomain: f.SendingDomain,
			DedupID:       messageID + "/" + email,
		})
	})
}

// confirmSNSSubscription confirms an SNS subscription by GETting the SubscribeURL.
// The URL must be an https amazonaws.com host (the payload is already signature-
// verified), and the fetch goes through the SSRF-hardened client.
func confirmSNSSubscription(ctx context.Context, subscribeURL string) error {
	u, err := url.Parse(subscribeURL)
	if err != nil {
		return fmt.Errorf("parse SubscribeURL: %w", err)
	}
	if u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".amazonaws.com") {
		return fmt.Errorf("refusing to confirm non-AWS SubscribeURL %q", subscribeURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subscribeURL, nil)
	if err != nil {
		return err
	}
	resp, err := webhook.NewClient(10 * time.Second).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("confirm subscription: status %d", resp.StatusCode)
	}
	return nil
}
