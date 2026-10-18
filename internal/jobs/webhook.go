package jobs

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/jobkind"
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

func (DeliverWebhookArgs) Kind() string { return jobkind.DeliverWebhook }

type DeliverWebhookWorker struct {
	river.WorkerDefaults[DeliverWebhookArgs]
	ent    *ent.Client
	cipher *secrets.Cipher
	client webhook.Doer
	now    func() time.Time
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
	failures := 1
	if e.ConsecutiveFailures != nil {
		failures = *e.ConsecutiveFailures + 1
	}
	u := s.WebhookEndpoint().UpdateOneID(e.ID).
		SetConsecutiveFailures(failures).
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
	return u.Exec(ctx)
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
