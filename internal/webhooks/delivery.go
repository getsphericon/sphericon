package webhooks

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// An endpoint disables itself (ADR 0039) on a 410, or when a run of consecutive failures has
// both lasted the window and reached the count. The window is what makes the count safe: a
// restart in the middle of a burst piles up failures in minutes and never retires a healthy
// endpoint.
const (
	DisableFailures = 20
	DisableWindow   = 2 * time.Hour
)

// Notify mails the Owners of a Workspace through the system sender. A nil Notify sends nothing.
type Notify func(ctx context.Context, workspaceID int64, subject, body string) error

// RecordDelivery keeps the endpoint's failure state after one delivery attempt (ADR 0039): a
// failed delivery extends the run of failures with its closed error kind, a 2xx clears it, and
// a 429 (the receiver is alive and asks us to slow down) is neither and leaves the counters
// alone. A failure that meets the disable rule turns the endpoint off. e is the row as the
// delivery loaded it; s is the endpoint's Workspace scope. The counter writes carry no actor,
// so they are not audited.
func (m *Module) RecordDelivery(ctx context.Context, s *ent.Scoped, e *ent.WebhookEndpoint, sendErr error, now time.Time) error {
	if sendErr == nil {
		if e.ConsecutiveFailures == nil {
			return nil
		}
		return s.WebhookEndpoint().UpdateOneID(e.ID).
			ClearConsecutiveFailures().ClearFailingSince().ClearLastErrorKind().
			ClearLastErrorStatus().ClearLastFailureAt().Exec(ctx)
	}
	var failure *webhook.DeliveryError
	if !errors.As(sendErr, &failure) {
		return nil // not a delivery outcome (an unusable secret): nothing to remember
	}
	if webhook.RateLimited(sendErr) != nil {
		return nil
	}
	// The count is incremented in SQL, so workers failing at once do not lose one.
	u := s.WebhookEndpoint().UpdateOneID(e.ID).
		AddConsecutiveFailures(1).
		SetLastErrorKind(webhookendpoint.LastErrorKind(failure.Kind)).
		SetLastFailureAt(now)
	if e.FailingSince == nil {
		u.SetFailingSince(now)
	}
	if failure.Kind == webhook.KindHTTPStatus {
		u.SetLastErrorStatus(failure.Status)
	} else {
		u.ClearLastErrorStatus()
	}
	updated, err := u.Save(ctx)
	if err != nil {
		return err
	}
	if reason, ok := disableReason(updated, failure, now); ok {
		return m.disable(ctx, s, updated, reason)
	}
	return nil
}

// disableReason applies the rule to the endpoint as its failure was just recorded.
func disableReason(e *ent.WebhookEndpoint, failure *webhook.DeliveryError, now time.Time) (webhookendpoint.DisabledReason, bool) {
	if failure.Kind == webhook.KindHTTPStatus && failure.Status == http.StatusGone {
		return webhookendpoint.DisabledReasonGone, true
	}
	if e.ConsecutiveFailures != nil && *e.ConsecutiveFailures >= DisableFailures &&
		e.FailingSince != nil && now.Sub(*e.FailingSince) >= DisableWindow {
		return webhookendpoint.DisabledReasonPersistentFailure, true
	}
	return "", false
}

// disable turns the endpoint off as the system actor, so the Audit log shows it, and mails
// the Workspace's owners once: the update only matches while the endpoint is still
// enabled, so a concurrent worker or a retry that lost the race sends nothing.
func (m *Module) disable(ctx context.Context, s *ent.Scoped, e *ent.WebhookEndpoint, reason webhookendpoint.DisabledReason) error {
	sys := m.bus.Act(s, events.Actor{Kind: events.ActorSystem})
	err := sys.WebhookEndpoint().UpdateOneID(e.ID).Where(webhookendpoint.Enabled(true)).
		SetEnabled(false).SetDisabledReason(reason).Exec(ctx)
	if ent.IsNotFound(err) {
		return nil // already disabled
	}
	if err != nil {
		return err
	}
	// A failed notice is logged, not returned: river would retry a delivery whose
	// endpoint is already off.
	if err := m.notifyOwners(ctx, s, e, reason); err != nil {
		slog.ErrorContext(ctx, "webhook disable notice failed", "endpoint_id", e.ID, "error", err)
	}
	return nil
}

func (m *Module) notifyOwners(ctx context.Context, s *ent.Scoped, e *ent.WebhookEndpoint, reason webhookendpoint.DisabledReason) error {
	if m.notify == nil {
		return nil
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return err
	}
	data := map[string]any{
		"Workspace": ws.Name,
		"URL":       originOf(e.URL),
		"Failures":  DisableFailures,
		"Window":    DisableWindow.String(),
	}
	data["Reason"] = i18n.T("email.webhook_disabled.reason_"+string(reason), data)
	return m.notify(ctx, ws.ID, i18n.T("email.webhook_disabled.subject", data), i18n.T("email.webhook_disabled.body", data))
}

// originOf keeps scheme and host of an endpoint URL for a mail: the path and query may
// carry a secret.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}
