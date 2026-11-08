package events_test

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/jobs"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// hardBounce is a hard bounce of email in the Workspace, as a provider's notice publishes it.
func hardBounce(workspaceID, contactID int64, email, dedupID string) *events.EmailDelivery {
	return &events.EmailDelivery{
		Action: events.NameEmailBounced, WorkspaceID: workspaceID, ContactID: contactID, Email: email,
		BounceKind: events.BounceKindHard, DedupID: dedupID,
	}
}

// suppressJob is the suppression consumer's job for a hard bounce of email, as Publish inserts it.
func suppressJob(t *testing.T, workspaceID, contactID int64, email, id string) jobs.SuppressEventArgs {
	t.Helper()
	return jobs.SuppressEventArgs{
		ID: id, Name: events.NameEmailBounced, Version: 1, WorkspaceID: workspaceID,
		Data: dataFor(t, hardBounce(workspaceID, contactID, email, id)),
	}
}

// Erasure removes the event jobs that name the Contact, by id or by address, queued and discarded
// alike; a running one is left to finish, and a job of another Contact or another Workspace stays.
func TestPurgeEventJobsRemovesTheJobsThatNameTheContact(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	kind := jobs.SuppressEventArgs{}.Kind()
	publishAlone(t, env, hardBounce(fixtures.InitechID, fixtures.ContactErasableID, "queued@example.com", "queued"))
	env.EnqueueDiscardedJob(t, suppressJob(t, fixtures.InitechID, 0, "ERASABLE@example.com", "discarded"))
	env.EnqueueRunningJob(t, suppressJob(t, fixtures.InitechID, fixtures.ContactErasableID, "running@example.com", "running"))
	publishAlone(t, env, hardBounce(fixtures.InitechID, 0, "someone-else@example.com", "other-contact"))
	publishAlone(t, env, hardBounce(fixtures.AcmeID, 0, "erasable@example.com", "other-workspace"))
	before := env.EventJobs(t, kind)
	require.Len(t, before, 5)

	scope := env.DB.Scoped(fixtures.InitechID)
	require.NoError(t, env.Bus.WithinScopedPurgeTx(ctx, scope, func(_ *ent.Scoped, pub events.PurgingPublisher) error {
		return pub.PurgeEventJobs(ctx, fixtures.InitechID, fixtures.ContactErasableID, []string{"erasable@example.com"})
	}))

	left := lo.Map(env.EventJobs(t, kind), func(e events.Envelope, _ int) string { return e.SourceID() })
	assert.ElementsMatch(t, []string{"running", "email.bounced:other-contact", "email.bounced:other-workspace"}, left)
	fanOuts := lo.Map(env.EventJobs(t, jobs.WebhookEventArgs{}.Kind()), func(e events.Envelope, _ int) string { return e.SourceID() })
	assert.ElementsMatch(t, []string{"email.bounced:other-contact", "email.bounced:other-workspace"}, fanOuts, "every event kind is purged, the webhooks fan-out included")
}
