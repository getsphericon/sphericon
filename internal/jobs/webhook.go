package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/webhookendpoints"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/httpcall"
	"github.com/getsphericon/sphericon/internal/infra/webhook"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
)

// DeliverWebhookArgs is one delivery attempt of an event to one endpoint. river
// retries on error, so a receiver may see duplicates — DeliveryID lets it dedupe. It is unique on
// the endpoint and the delivery, so the deliveries a retried dispatch queues again are the ones
// already queued.
type DeliverWebhookArgs struct {
	jobkind.Workspace

	EndpointID int64  `json:"endpoint_id" river:"unique"`
	EventName  string `json:"event_name"`
	DeliveryID string `json:"delivery_id" river:"unique"`
	Body       []byte `json:"body"`
}

func (DeliverWebhookArgs) Kind() string { return jobkind.DeliverWebhook.Kind }

func (DeliverWebhookArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       QueueWebhooks,
		MaxAttempts: webhook.HookMaxAttempts(),
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

type DeliverWebhookWorker struct {
	hookRetries[DeliverWebhookArgs]

	ent    *ent.Client
	client webhook.Doer
	now    func() time.Time
	// mod keeps the endpoint's failure state and disables it, which tells the owners.
	mod *webhookendpoints.Module
}

// NewDeliverWebhookWorker builds the worker that delivers a webhook through doer on the clock now,
// spreading its retries by jitter; an endpoint the delivery turns off publishes it on bus, which
// tells the owners (ADR 0105).
func NewDeliverWebhookWorker(client *ent.Client, cipher *secrets.Cipher, doer webhook.Doer, now func() time.Time, jitter webhook.HookJitter, bus *events.Bus) *DeliverWebhookWorker {
	return &DeliverWebhookWorker{
		jitter: jitter,
		ent:    client,
		client: doer,
		now:    now,
		mod:    webhookendpoints.New(bus, cipher, nil, nil, now),
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
	delivery, ok, err := w.mod.DeliveryTo(e)
	if err != nil || !ok {
		return err // !ok: disabled since enqueue
	}
	sendErr := webhook.Send(ctx, w.client, delivery.URL, delivery.Secret, job.Args.EventName, job.Args.DeliveryID, job.Args.Body, w.now())
	if err := w.mod.RecordDelivery(ctx, w.ent.Scoped(e.WorkspaceID), e, sendErr, w.now()); err != nil {
		return err
	}
	if sendErr == nil {
		return nil
	}
	decision := httpcall.Subscription.Decide(sendErr, hookAttempt(job.JobRow), w.now())
	switch {
	case decision.Outcome == httpcall.Wait:
		return river.JobSnooze(decision.Wait)
	case decision.Outcome == httpcall.GiveUp && decision.Reason != httpcall.AttemptsExhausted:
		return river.JobCancel(sendErr) // a receiver that never lets up ends the delivery too
	}
	// A retry, or the last attempt, which river ends as exhausted.
	return sendErr
}

// NewWebhookDispatcher is the dispatcher that queues the deliveries of an event through the river
// client of the job it runs in.
func NewWebhookDispatcher() events.WebhookDispatcher {
	return webhookDispatcher{}
}

type webhookDispatcher struct{}

// Dispatch fans a domain event out to every endpoint of the workspace that receives it
// (webhookendpoints.Subscribed), enqueuing one delivery job each, all in one insert. The scope
// comes from the caller (the webhooks consumer, which scopes to the event's Workspace).
func (d webhookDispatcher) Dispatch(ctx context.Context, s *ent.Scoped, eventName, deliveryID string, body []byte) error {
	endpoints, err := webhookendpoints.Subscribed(ctx, s, eventName)
	if err != nil {
		return err
	}
	return queueEach(ctx, endpoints, func(e *ent.WebhookEndpoint) river.JobArgs {
		return DeliverWebhookArgs{
			WorkspaceID: e.WorkspaceID,
			EndpointID:  e.ID,
			EventName:   eventName,
			DeliveryID:  deliveryID,
			Body:        body,
		}
	})
}
