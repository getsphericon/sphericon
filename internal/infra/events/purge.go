package events

import (
	"context"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/internal/kernel/publicid"

	"github.com/riverqueue/river"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/eventjournalentry"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/ent/workspace"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
)

// QueuePurger removes what the river job table still holds about a data subject. The Publisher
// handed to Bus.WithinTx implements it over the same transaction, so a purge commits (or rolls
// back) with the state change that asks for it. Erasure is its only caller (ADR 0021); it is a
// separate interface so ordinary producers cannot reach it.
//
// The job deletes go through river's own delete (JobDeleteManyTx) on the same transaction;
// a Where predicate matches the args.
type QueuePurger interface {
	// PurgeEventJobs deletes the Workspace's event consumer jobs (QueueEvents) whose envelope names
	// the Contact by id or by one of its destinations, an address (matched case-insensitively) or a
	// phone number, in any state but running: queued, retrying and discarded ones alike. A running
	// (or locked) job is left to finish: a consumer that loads the Contact finds it gone, and one
	// that only writes into the Workspace (the suppression, the orders, the Audit log, the Erasures
	// stream) runs to its end.
	PurgeEventJobs(ctx context.Context, workspaceID, contactID int64, destinations []string) error
	// PurgeJobs deletes the river jobs of kind whose args integer field key is one of ids.
	// A running job, or one a fetch holds locked to start it, is left to finish: each of these
	// kinds' workers loads the row its args name and ends when it is gone.
	PurgeJobs(ctx context.Context, key jobkind.Key, ids []int64) error
	// PurgeWebhookJobs deletes the queued deliveries to the Workspace's own webhook
	// endpoints whose body names the Contact (contactId) or one of its
	// destinations (data.email case-insensitively, or data.phone). The body is the event payload
	// the endpoint would receive. A running (or locked) delivery is left to finish.
	PurgeWebhookJobs(ctx context.Context, workspaceID, contactID int64, destinations []string) error
}

// purgeBatch is how many jobs one delete of a purge removes. river deletes at most a batch per
// call (10000 at most), so a purge deletes again until a batch comes back short.
const purgeBatch = 1000

func (p *txPublisher) PurgeJobs(ctx context.Context, key jobkind.Key, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return p.deleteJobs(ctx, river.NewJobDeleteManyParams().Kinds(key.Kind).
		// raw sql: an integer field of the args matched against a list; river's delete filters
		// stop at a job's own columns.
		Where("(args->>@purge_arg::text)::bigint = any(@purge_ids::bigint[])",
			river.NamedArgs{"purge_arg": key.Arg, "purge_ids": ids}))
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
		id = publicid.Encode(contactID) // the body names the Contact by its public id (ADR 0142)
	}
	// DeliverWebhookArgs.Body is []byte, so river stores it as a base64 JSON string; it is the
	// JSON event payload, naming the Contact at contactId / data.contactId, the address at
	// data.email, the phone number at data.phone.
	return p.deleteJobs(ctx, river.NewJobDeleteManyParams().Kinds(jobkind.DeliverWebhook.Kind).
		// raw sql: an integer field of the args matched against a list; river's delete filters
		// stop at a job's own columns.
		Where("(args->>@purge_endpoint_arg::text)::bigint = any(@purge_endpoints::bigint[])",
			river.NamedArgs{"purge_endpoint_arg": jobkind.DeliverWebhook.Arg, "purge_endpoints": endpointIDs}).
		// raw sql: decodes a base64 JSON string of the args into a document and matches inside it.
		Where("EXISTS (SELECT 1 FROM (SELECT convert_from(decode(args->>@purge_body_arg::text, 'base64'), 'UTF8')::jsonb AS body) b"+
			" WHERE b.body->>'contactId' = @purge_contact::text OR b.body->'data'->>'contactId' = @purge_contact::text"+
			" OR lower(b.body->'data'->>'email') = any(@purge_addresses::text[])"+
			" OR b.body->'data'->>'phone' = any(@purge_addresses::text[]))",
			river.NamedArgs{"purge_body_arg": jobkind.DeliverWebhookBodyArg, "purge_contact": id, "purge_addresses": lowerAll(destinations)}))
}

func (p *txPublisher) PurgeEventJobs(ctx context.Context, workspaceID, contactID int64, destinations []string) error {
	id := ""
	if contactID != 0 {
		id = strconv.FormatInt(contactID, 10) // an event job keeps the envelope, which holds the key
	}
	// An event job's args are its envelope (EventJob): the Workspace at workspaceId, the event at
	// data, which names the Contact as contactId, the address as email and the phone as phone.
	return p.deleteJobs(ctx, river.NewJobDeleteManyParams().Queues(QueueEvents).
		// raw sql: a field of the args matched against a value; river's delete filters stop at a
		// job's own columns.
		Where("args->>'workspaceId' = @purge_workspace::text",
			river.NamedArgs{"purge_workspace": strconv.FormatInt(workspaceID, 10)}).
		// raw sql: the event's fields inside the args, the address case-insensitively.
		Where("(args->'data'->>'contactId' = @purge_event_contact::text"+
			" OR lower(args->'data'->>'email') = any(@purge_event_addresses::text[])"+
			" OR args->'data'->>'phone' = any(@purge_event_addresses::text[]))",
			river.NamedArgs{"purge_event_contact": id, "purge_event_addresses": lowerAll(destinations)}))
}

// deleteJobs deletes the jobs params selects through river, on the purge's transaction, a batch
// at a time until a batch comes back short. river never deletes a running job, nor one that is
// locked (a fetch about to start it): either is left to finish, and what its worker does then is
// for each purge to say (QueuePurger, WorkspacePurger).
func (p *txPublisher) deleteJobs(ctx context.Context, params *river.JobDeleteManyParams) error {
	if p.jobs == nil {
		return ErrNoJobQueue
	}
	params = params.First(purgeBatch)
	for {
		res, err := p.jobs.JobDeleteManyTx(ctx, p.tx, params)
		if err != nil {
			return err
		}
		if len(res.Jobs) < purgeBatch {
			return nil
		}
	}
}

func lowerAll(in []string) []string {
	out := make([]string, len(in))
	for i, d := range in {
		out[i] = strings.ToLower(strings.TrimSpace(d))
	}
	return out
}

var _ QueuePurger = (*txPublisher)(nil)

// WorkspacePurger removes what the river job table still holds of a Workspace. Only
// WithinWorkspacePurgeTx hands one out: the erase of a deleted Workspace is its one caller, so no
// producer and no Contact's Erasure can reach it.
type WorkspacePurger interface {
	// PurgeWorkspaceJobs deletes every job of the Workspace, whatever its kind or queue, in any
	// state but running: available, scheduled, retryable, completed, cancelled and discarded
	// alike. A job's Workspace is the args' top-level jobkind.WorkspaceArg, which every job of a
	// Workspace carries (jobkind.Workspace, an event job's envelope); a job of the Instance names
	// none and stays. A running (or locked) job is left to finish, and one that runs again later
	// (a retry) is cancelled before its worker starts, its Workspace being erased (the job
	// runner's worker middleware, jobs.WorkerConfig). Only a job already inside its worker when
	// the tombstone commits can still write a row: a consumer that writes into the envelope's
	// Workspace without loading a row of it (the suppression, the orders, the Audit log, the
	// Erasures stream).
	PurgeWorkspaceJobs(ctx context.Context, workspaceID int64) error
}

// WithinWorkspacePurgeTx is WithinTx for the last transaction of a deleted Workspace's erase
// (workspacedeletion), which removes the Workspace's jobs with the rest of what is left of it, so
// the purge commits (or rolls back) with the tombstone.
func (b *Bus) WithinWorkspacePurgeTx(ctx context.Context, fn func(tx *ent.Client, purger WorkspacePurger) error) error {
	return b.within(ctx, func(tx *ent.Client, pub *txPublisher) error { return fn(tx, pub) })
}

func (p *txPublisher) PurgeWorkspaceJobs(ctx context.Context, workspaceID int64) error {
	// Fence publications and replays before deleting their history and jobs. Once this
	// lock is acquired, earlier publishers committed and the following delete sees them;
	// later ones wait for the tombstone and cannot recreate journal history afterwards.
	if _, err := p.client.Workspace.Query().Where(workspace.ID(workspaceID)).ForUpdate().All(ctx); err != nil {
		return err
	}
	if _, err := p.client.EventJournalEntry.Delete().Where(eventjournalentry.WorkspaceID(workspaceID)).Exec(ctx); err != nil {
		return err
	}
	return p.deleteJobs(ctx, river.NewJobDeleteManyParams().
		// raw sql: a field of the args matched by containment, which river's GIN index on args
		// serves; river's delete filters stop at a job's own columns.
		Where("args @> jsonb_build_object(@purge_workspace_arg::text, @purge_workspace_id::bigint)",
			river.NamedArgs{"purge_workspace_arg": jobkind.WorkspaceArg, "purge_workspace_id": workspaceID}))
}

var _ WorkspacePurger = (*txPublisher)(nil)
