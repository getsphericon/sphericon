package jobs

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/domain/identity/memberships"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// DeliverWebhookArgs is one delivery attempt of an event to one endpoint. river
// retries on error, so a receiver may see duplicates — DeliveryID lets it dedupe.
type DeliverWebhookArgs struct {
	EndpointID int64  `json:"endpoint_id"`
	EventName  string `json:"event_name"`
	DeliveryID string `json:"delivery_id"`
	Body       []byte `json:"body"`
}

func (DeliverWebhookArgs) Kind() string { return jobkind.DeliverWebhook.Kind }

type DeliverWebhookWorker struct {
	river.WorkerDefaults[DeliverWebhookArgs]

	ent    *ent.Client
	cipher *secrets.Cipher
	client webhook.Doer
	now    func() time.Time
	// mod keeps the endpoint's failure state, disables it and tells the owners.
	mod *webhookendpoints.Module
}

// NewDeliverWebhookWorker builds the worker that delivers a webhook through doer on the clock now;
// the owners of a Workspace whose endpoint is turned off are told through sender.
func NewDeliverWebhookWorker(client *ent.Client, cipher *secrets.Cipher, doer webhook.Doer, now func() time.Time, bus *events.Bus, sender messaging.EmailSender) *DeliverWebhookWorker {
	return &DeliverWebhookWorker{
		ent:    client,
		cipher: cipher,
		client: doer,
		now:    now,
		mod:    webhookendpoints.New(bus, cipher, nil, nil, memberships.OwnerNotifier(client, sender), now),
	}
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
	if e.DisabledAt != nil {
		return nil // disabled since enqueue
	}

	secret, err := w.cipher.Decrypt(e.SecretEncrypted)
	if err != nil {
		return err
	}
	sendErr := webhook.Send(ctx, w.client, e.URL, string(secret), job.Args.EventName, job.Args.DeliveryID, job.Args.Body, w.now())
	if err := w.mod.RecordDelivery(ctx, w.ent.Scoped(e.WorkspaceID), e, sendErr, w.now()); err != nil {
		return err
	}
	if verdict, limited := webhook.DecideRateLimited(sendErr, job.CreatedAt, w.now()); limited {
		if verdict.GiveUp {
			return river.JobCancel(sendErr) // a receiver that never lets up ends the delivery too
		}
		return river.JobSnooze(verdict.Wait)
	}
	return sendErr
}

// NextRetry is the delivery's retry policy (webhook.HookNextRetry), so a failed delivery is retried
// for days rather than river's default hours; Dispatch inserts with the matching MaxAttempts.
func (w *DeliverWebhookWorker) NextRetry(job *river.Job[DeliverWebhookArgs]) time.Time {
	if job.AttemptedAt == nil {
		return time.Time{} // river falls back to its default
	}
	return webhook.HookNextRetry(*job.AttemptedAt, job.Attempt)
}

// NewWebhookDispatcher is the dispatcher that queues the deliveries of an event through the river
// client of the job it runs in.
func NewWebhookDispatcher() events.WebhookDispatcher {
	return webhookDispatcher{}
}

type webhookDispatcher struct{}

// Dispatch fans a domain event out to every enabled endpoint in the workspace
// whose filter matches, enqueuing one delivery job each. The scope comes from the caller
// (the webhooks consumer, which scopes to the event's Workspace).
func (d webhookDispatcher) Dispatch(ctx context.Context, s *ent.Scoped, eventName, deliveryID string, body []byte) error {
	endpoints, err := s.WebhookEndpoint().Query().
		Where(webhookendpoint.DisabledAtIsNil()).
		All(ctx)
	if err != nil {
		return err
	}
	queue := river.ClientFromContext[*sql.Tx](ctx)
	for _, e := range endpoints {
		if !matchesEvent(e.EventTypes, eventName) {
			continue
		}
		if _, err := queue.Insert(ctx, DeliverWebhookArgs{
			EndpointID: e.ID,
			EventName:  eventName,
			DeliveryID: deliveryID,
			Body:       body,
		}, &river.InsertOpts{Queue: QueueWebhooks, MaxAttempts: webhook.HookMaxAttempts()}); err != nil {
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
