package events

import (
	"context"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent/outboxmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/jobkind"
)

// QueuePurger removes what the internal queues still hold about a data subject: the
// domain-event outbox and the river job table. The Publisher handed to
// Bus.WithinTx implements it over the same transaction, so a purge commits (or
// rolls back) with the state change that asks for it. Erasure is its only caller
// (ADR 0021); it is a separate interface so ordinary producers cannot reach it.
//
// The job deletes are SQL on river_job on purpose. river's Tx API would work here
// (riverdatabasesql v0.49.0 matches river, and JobListTx/JobDeleteManyTx run on this
// *sql.Tx), but JobDeleteManyParams has no args predicate and JobListParams only an
// args-capable Where(sql, args) escape hatch, so the match would still be this same
// SQL, wrapped in a second river client, a new driver dependency and a list-then-delete
// loop. The tests' own queue access does use river's API (internal/testhelper/jobs.go).
type QueuePurger interface {
	// PurgeOutbox deletes the Workspace's outbox envelopes that name the Contact,
	// by id or by one of its destinations (matched case-insensitively).
	PurgeOutbox(ctx context.Context, workspaceID, contactID int64, destinations []string) error
	// PurgeJobs deletes the river jobs of kind that are not running and whose
	// args integer field key is one of ids. A running job is left to finish: its
	// worker re-checks that what it refers to still exists.
	PurgeJobs(ctx context.Context, key jobkind.Key, ids []int64) error
	// PurgeWebhookJobs deletes the queued deliveries to the Workspace's own webhook
	// endpoints whose body names the Contact (contactId) or one of its
	// destinations (data.email, case-insensitively). The body is the event payload
	// the endpoint would receive. A running delivery is left to finish.
	PurgeWebhookJobs(ctx context.Context, workspaceID, contactID int64, destinations []string) error
}

func (p *txPublisher) PurgeOutbox(ctx context.Context, workspaceID, contactID int64, destinations []string) error {
	lowered := lowerAll(destinations)
	// contactID 0 is "no Contact" (an address erased on its own); events with no
	// Contact carry contactId 0 or none, so 0 must never match.
	id := ""
	if contactID != 0 {
		id = fmt.Sprint(contactID)
	}
	_, err := p.client.OutboxMessage.Delete().Where(outboxNamesSubject(workspaceID, id, lowered)).Exec(ctx)
	return err
}

// outboxNamesSubject selects a Workspace's outbox envelopes that name a data subject. The
// payload is the JSON-encoded Envelope; events name the Contact as data.contactId and the
// address as data.email. ent has no predicate over a JSON document, so this is one on
// the generated query.
func outboxNamesSubject(workspaceID int64, contactID string, lowered []string) predicate.OutboxMessage {
	return predicate.OutboxMessage(func(s *sql.Selector) {
		payload := s.C(outboxmessage.FieldPayload)
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("(" + payload + "::jsonb->>'workspaceId')::bigint = ").Arg(workspaceID).
				WriteString(" AND ((" + payload + "::jsonb->'data'->>'contactId') = ").Arg(contactID).
				WriteString(" OR lower(" + payload + "::jsonb->'data'->>'email') = ANY(").Arg(lowered).WriteString("))")
		}))
	})
}

func (p *txPublisher) PurgeJobs(ctx context.Context, key jobkind.Key, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := p.tx.ExecContext(ctx, `DELETE FROM river_job
WHERE kind = $1 AND state <> 'running' AND (args->>$2)::bigint = ANY($3)`,
		key.Kind, key.Arg, ids)
	return err
}

func (p *txPublisher) PurgeWebhookJobs(ctx context.Context, workspaceID, contactID int64, destinations []string) error {
	endpointIDs, err := p.client.WebhookEndpoint.Query().Where(webhookendpoint.WorkspaceID(workspaceID)).IDs(ctx)
	if err != nil {
		return err
	}
	if len(endpointIDs) == 0 {
		return nil
	}
	id := ""
	if contactID != 0 {
		id = fmt.Sprint(contactID)
	}
	// DeliverWebhookArgs.Body is []byte, so river stores it as a base64 JSON string; it
	// is the JSON event payload, naming the Contact at contactId / data.contactId and
	// the address at data.email.
	_, err = p.tx.ExecContext(ctx, `DELETE FROM river_job
WHERE kind = $1 AND state <> 'running'
  AND (args->>$5)::bigint = ANY($2)
  AND EXISTS (
    SELECT 1 FROM (SELECT convert_from(decode(args->>$6, 'base64'), 'UTF8')::jsonb AS body) b
    WHERE b.body->>'contactId' = $3
       OR b.body->'data'->>'contactId' = $3
       OR lower(b.body->'data'->>'email') = ANY($4))`,
		jobkind.DeliverWebhook.Kind, endpointIDs, id, lowerAll(destinations),
		jobkind.DeliverWebhook.Arg, jobkind.DeliverWebhookBodyArg)
	return err
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, d := range in {
		out[i] = strings.ToLower(strings.TrimSpace(d))
	}
	return out
}

var _ QueuePurger = (*txPublisher)(nil)
