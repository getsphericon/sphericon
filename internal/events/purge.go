package events

import (
	"context"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"github.com/getsphericon/sphericon/ent/outboxmessage"
	"github.com/getsphericon/sphericon/ent/predicate"
	"github.com/getsphericon/sphericon/ent/riverjob"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/jobkind"
)

// QueuePurger removes what the internal queues still hold about a data subject: the
// domain-event outbox and the river job table. The Publisher handed to
// Bus.WithinTx implements it over the same transaction, so a purge commits (or
// rolls back) with the state change that asks for it. Erasure is its only caller
// (ADR 0021); it is a separate interface so ordinary producers cannot reach it.
//
// The job deletes go through the RiverJob skipped entity (river's table; ent only
// generates its client): river's Tx API has no args predicate, so ent matches the args.
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
// address as data.email. The paths are ent's JSON predicates; only the case-insensitive
// address match is written by hand (the lower() of a JSON path has no predicate), around
// ent's own path expression.
func outboxNamesSubject(workspaceID int64, contactID string, lowered []string) predicate.OutboxMessage {
	return predicate.OutboxMessage(func(s *sql.Selector) {
		payload := s.C(outboxmessage.FieldPayload)
		s.Where(sql.And(
			sqljson.ValueEQ(payload, workspaceID, sqljson.Path("workspaceId"), sqljson.Cast("bigint")),
			sql.Or(
				sqljson.ValueEQ(payload, contactID, sqljson.Path("data", "contactId")),
				sql.P(func(b *sql.Builder) {
					b.WriteString("lower(").
						Join(sqljson.ValuePath(payload, sqljson.Path("data", "email"), sqljson.Unquote(true))).
						WriteString(") = ANY(").Arg(lowered).WriteString(")")
				}),
			),
		))
	})
}

// argInts selects river jobs whose args integer field key is one of ids.
func argInts(key string, ids []int64) predicate.RiverJob {
	return predicate.RiverJob(func(s *sql.Selector) {
		vals := make([]any, len(ids))
		for i, id := range ids {
			vals[i] = id
		}
		s.Where(sqljson.ValueIn(s.C(riverjob.FieldArgs), vals, sqljson.Path(key), sqljson.Cast("bigint")))
	})
}

// riverStateRunning is river's river_job_state 'running'. A running job is left to
// finish: its worker re-checks that what it refers to still exists.
const riverStateRunning = "running"

func (p *txPublisher) PurgeJobs(ctx context.Context, key jobkind.Key, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := p.client.RiverJob.Delete().
		Where(riverjob.Kind(key.Kind), riverjob.StateNEQ(riverStateRunning), argInts(key.Arg, ids)).
		Exec(ctx)
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
	lowered := lowerAll(destinations)
	// DeliverWebhookArgs.Body is []byte, so river stores it as a base64 JSON string; it
	// is the JSON event payload, naming the Contact at contactId / data.contactId and
	// the address at data.email. No ent predicate decodes base64 into a JSON document, so
	// the body match is written by hand; the rest of the selection is ent's.
	namesSubject := predicate.RiverJob(func(s *sql.Selector) {
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("EXISTS (SELECT 1 FROM (SELECT convert_from(decode(").
				Join(sqljson.ValuePath(s.C(riverjob.FieldArgs), sqljson.Path(jobkind.DeliverWebhookBodyArg), sqljson.Unquote(true))).
				WriteString(", 'base64'), 'UTF8')::jsonb AS body) b WHERE b.body->>'contactId' = ").Arg(id).
				WriteString(" OR b.body->'data'->>'contactId' = ").Arg(id).
				WriteString(" OR lower(b.body->'data'->>'email') = ANY(").Arg(lowered).WriteString("))")
		}))
	})
	_, err = p.client.RiverJob.Delete().Where(
		riverjob.Kind(jobkind.DeliverWebhook.Kind), riverjob.StateNEQ(riverStateRunning),
		argInts(jobkind.DeliverWebhook.Arg, endpointIDs), namesSubject,
	).Exec(ctx)
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
