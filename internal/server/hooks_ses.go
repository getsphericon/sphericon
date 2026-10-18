package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/logging"
	"github.com/getsphericon/sphericon/internal/preset"
	"github.com/getsphericon/sphericon/internal/webhook"
	sns "github.com/robbiet480/go.sns"
)

// The SES adapter (AWS SES delivered over SNS) reads an SES Integration's callbacks until
// the `ses` Preset replaces it (ADR 0037). It is hand-written: it verifies the SNS signature,
// refuses a topic the Integration did not name, confirms a subscription and normalizes the
// notification into Delivery failures that hook.publish writes to the outbox.

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

func (h *hook) serveSES(w http.ResponseWriter, r *http.Request, row *ent.Integration, body []byte) {
	var payload sns.Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid SNS payload")
		return
	}
	if err := h.verifySNS(&payload); err != nil {
		writeProblem(w, http.StatusForbidden, "SNS signature verification failed")
		return
	}

	// A valid signature proves AWS sent this, not whose topic it came from: the topic must
	// be the one this Integration named, or anyone holding the key could suppress any address.
	topic, err := h.integ.SNSTopic(row)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "integration lookup failed")
		return
	}
	if topic == "" {
		writeProblem(w, http.StatusForbidden, "no SNS topic is configured: set snsTopicArn on the SES Integration")
		return
	}
	if payload.TopicArn != topic {
		writeProblem(w, http.StatusForbidden, "SNS topic is not the one configured on the SES Integration")
		return
	}

	switch payload.Type {
	case "SubscriptionConfirmation":
		// Confirm the subscription by fetching the (SSRF-guarded) SubscribeURL. On
		// failure return 5xx so SNS resends the confirmation.
		if err := h.confirmURL(r.Context(), payload.SubscribeURL); err != nil {
			logging.FromContext(r.Context()).Error("hooks/ses: confirm subscription failed", "integration_id", row.ID, "err", err)
			writeProblem(w, http.StatusBadGateway, "subscription confirmation failed")
			return
		}
	case "Notification":
		// On failure return 5xx so SNS redelivers rather than dropping the bounce;
		// downstream persist/suppression dedupe on redelivery (DedupID).
		s := events.Ingest(h.ent.Scoped(row.WorkspaceID))
		if err := h.handleSESNotification(r.Context(), s, payload); err != nil {
			logging.FromContext(r.Context()).Error("hooks/ses: notification processing failed", "integration_id", row.ID, "err", err)
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

func (h *hook) handleSESNotification(ctx context.Context, s *ent.Scoped, payload sns.Payload) error {
	failures, err := parseSESNotification(payload.Message)
	if err != nil {
		return err
	}
	out := make([]preset.DeliveryFailure, 0, len(failures))
	for _, f := range failures {
		df := preset.DeliveryFailure{
			ID: payload.MessageId, Destination: f.Email, SendingDomain: f.SendingDomain,
			Kind: preset.Bounce, Permanent: f.Kind == events.BounceKindPermanent,
		}
		if f.Action == events.NameEmailComplained {
			df.Kind = preset.Complaint
		}
		out = append(out, df)
	}
	return h.publish(ctx, s, "ses", out)
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
