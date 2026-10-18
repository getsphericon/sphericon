package jobs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/jobkind"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/webhook"
)

// DeliverWebhookArgs is one delivery attempt of an event to one endpoint. river
// retries on error, so a receiver may see duplicates — DeliveryID lets it dedupe.
type DeliverWebhookArgs struct {
	EndpointID int64  `json:"endpoint_id"`
	EventName  string `json:"event_name"`
	DeliveryID string `json:"delivery_id"`
	Body       []byte `json:"body"`
}

// A Webhook endpoint disables itself (ADR 0039) on a 410, or when a run of consecutive
// failures has both lasted the window and reached the count. The window is what makes the
// count safe: a restart in the middle of a burst piles up failures in minutes and never
// retires a healthy endpoint.
const (
	WebhookDisableFailures = 20
	WebhookDisableWindow   = 2 * time.Hour
)

func (DeliverWebhookArgs) Kind() string { return jobkind.DeliverWebhook }

type DeliverWebhookWorker struct {
	river.WorkerDefaults[DeliverWebhookArgs]
	ent    *ent.Client
	cipher *secrets.Cipher
	client webhook.Doer
	now    func() time.Time
	// bus and sender serve the automatic disable: an audited change by the system actor,
	// and the notice to the Workspace's owners through the platform sender.
	bus    *events.Bus
	sender messaging.EmailSender
}

func (w *DeliverWebhookWorker) Work(ctx context.Context, job *river.Job[DeliverWebhookArgs]) error {
	// Job entry point: the args carry only the endpoint id, so the Workspace is not
	// known up front and this one read uses the raw client (ADR 0017). Everything
	// after it needs only the loaded row; a further read would build its scope from
	// e.WorkspaceID.
	e, err := w.ent.WebhookEndpoint.Get(ctx, job.Args.EndpointID)
	if ent.IsNotFound(err) {
		return nil // endpoint deleted since enqueue; drop the delivery
	}
	if err != nil {
		return err
	}
	if !e.Enabled {
		return nil // disabled since enqueue
	}

	secret, err := w.cipher.Decrypt(e.SecretEncrypted)
	if err != nil {
		return err
	}
	sendErr := webhook.Send(ctx, w.client, e.URL, string(secret), job.Args.EventName, job.Args.DeliveryID, job.Args.Body)
	if err := w.recordOutcome(ctx, e, sendErr); err != nil {
		return err
	}
	return sendErr
}

// recordOutcome keeps the endpoint's failure state (ADR 0039): a failed delivery extends
// the run of failures with its closed error kind, a 2xx clears it. The scope has no actor,
// so these system writes are not audited.
func (w *DeliverWebhookWorker) recordOutcome(ctx context.Context, e *ent.WebhookEndpoint, sendErr error) error {
	s := w.ent.Scoped(e.WorkspaceID)
	var deliveryErr *webhook.DeliveryError
	if sendErr == nil {
		if e.ConsecutiveFailures == nil {
			return nil
		}
		return s.WebhookEndpoint().UpdateOneID(e.ID).
			ClearConsecutiveFailures().ClearFailingSince().ClearLastErrorKind().
			ClearLastErrorStatus().ClearLastFailureAt().Exec(ctx)
	}
	if !errors.As(sendErr, &deliveryErr) {
		return nil // not a delivery outcome (an unusable secret): nothing to remember
	}
	now := w.now()
	// The count is incremented in SQL, so workers failing at once do not lose one.
	u := s.WebhookEndpoint().UpdateOneID(e.ID).
		AddConsecutiveFailures(1).
		SetLastErrorKind(webhookendpoint.LastErrorKind(deliveryErr.Kind)).
		SetLastFailureAt(now)
	if e.FailingSince == nil {
		u.SetFailingSince(now)
	}
	if deliveryErr.Kind == webhook.KindHTTPStatus {
		u.SetLastErrorStatus(deliveryErr.Status)
	} else {
		u.ClearLastErrorStatus()
	}
	updated, err := u.Save(ctx)
	if err != nil {
		return err
	}
	if reason, ok := disableReason(updated, deliveryErr, now); ok {
		return w.disable(ctx, updated, reason)
	}
	return nil
}

// disableReason applies the rule to the endpoint as its failure was just recorded.
func disableReason(e *ent.WebhookEndpoint, failure *webhook.DeliveryError, now time.Time) (webhookendpoint.DisabledReason, bool) {
	if failure.Kind == webhook.KindHTTPStatus && failure.Status == http.StatusGone {
		return webhookendpoint.DisabledReasonGone, true
	}
	if e.ConsecutiveFailures != nil && *e.ConsecutiveFailures >= WebhookDisableFailures &&
		e.FailingSince != nil && now.Sub(*e.FailingSince) >= WebhookDisableWindow {
		return webhookendpoint.DisabledReasonPersistentFailure, true
	}
	return "", false
}

// disable turns the endpoint off as the system actor, so the Audit log shows it, and mails
// the Workspace's owners once: the update only matches while the endpoint is still
// enabled, so a concurrent worker or a retry that lost the race sends nothing.
func (w *DeliverWebhookWorker) disable(ctx context.Context, e *ent.WebhookEndpoint, reason webhookendpoint.DisabledReason) error {
	s := w.bus.Act(w.ent.Scoped(e.WorkspaceID), events.Actor{Kind: events.ActorSystem})
	err := s.WebhookEndpoint().UpdateOneID(e.ID).Where(webhookendpoint.Enabled(true)).
		SetEnabled(false).SetDisabledReason(reason).Exec(ctx)
	if ent.IsNotFound(err) {
		return nil // already disabled
	}
	if err != nil {
		return err
	}
	// A failed notice is logged, not returned: river would retry a delivery whose
	// endpoint is already off.
	if err := w.notifyOwners(ctx, e, reason); err != nil {
		slog.ErrorContext(ctx, "webhook disable notice failed", "endpoint_id", e.ID, "error", err)
	}
	return nil
}

func (w *DeliverWebhookWorker) notifyOwners(ctx context.Context, e *ent.WebhookEndpoint, reason webhookendpoint.DisabledReason) error {
	if w.sender == nil {
		return nil
	}
	ws, err := w.ent.Workspace.Get(ctx, e.WorkspaceID)
	if err != nil {
		return err
	}
	data := map[string]any{
		"Workspace": ws.Name,
		"URL":       originOf(e.URL),
		"Failures":  WebhookDisableFailures,
		"Window":    WebhookDisableWindow.String(),
	}
	data["Reason"] = i18n.T("email.webhook_disabled.reason_"+string(reason), data)
	return NotifyWorkspaceOwners(ctx, w.ent, w.sender, ws.ID,
		i18n.T("email.webhook_disabled.subject", data), i18n.T("email.webhook_disabled.body", data))
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

// Dispatch fans a domain event out to every enabled endpoint in the workspace
// whose filter matches, enqueuing one delivery job each. Implements
// events.WebhookDispatcher. The scope comes from the caller (the events bus
// subscriber, a raw-client allowlist entry).
func (c *Client) Dispatch(ctx context.Context, s *ent.Scoped, eventName, deliveryID string, body []byte) error {
	endpoints, err := s.WebhookEndpoint().Query().
		Where(webhookendpoint.Enabled(true)).
		All(ctx)
	if err != nil {
		return err
	}
	for _, e := range endpoints {
		if !matchesEvent(e.EventTypes, eventName) {
			continue
		}
		if _, err := c.river.Insert(ctx, DeliverWebhookArgs{
			EndpointID: e.ID,
			EventName:  eventName,
			DeliveryID: deliveryID,
			Body:       body,
		}, &river.InsertOpts{Queue: QueueWebhooks, MaxAttempts: 10}); err != nil {
			return err
		}
	}
	return nil
}

// matchesEvent reports whether an endpoint with the given filter receives the
// event. An empty filter means "all customer-facing events": an unprojected event
// (an Audit entry, ADR 0022) must be named explicitly.
func matchesEvent(filter []string, name string) bool {
	if len(filter) == 0 {
		return !events.IsUnprojected(name)
	}
	return slices.Contains(filter, name)
}
