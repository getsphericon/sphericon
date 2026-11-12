package events_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/systemevent"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func auditEntry(actorKind string) *events.AuditEntry {
	return &events.AuditEntry{
		WorkspaceID: fixtures.AcmeID,
		Actor:       events.Actor{Kind: actorKind, ID: "1", Name: "John"},
		Action:      "membership.update",
		TargetType:  membership.Label,
		TargetID:    "3",
	}
}

// An Audit entry is administrative history, not a data-plane fact: publishing it
// writes no Event row, so no segment or trigger can ever see it.
func TestAuditEntryIsNotProjectedToAnEvent(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	before, err := env.DB.Event.Query().Count(ctx)
	require.NoError(t, err)

	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		return events.RecordAudit(ctx, pub, auditEntry(events.ActorUser))
	}))
	published := env.QueuedEnvelopes(t, events.NameAuditEntry)
	require.Len(t, published, 1)

	after, err := env.DB.Event.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assert.True(t, events.IsUnprojected(events.NameAuditEntry))
	assert.False(t, events.IsUnprojected(systemevent.ContactCreated))
}

// The seam records only human, token, staff member and system actors: ingest (and no
// actor) never reaches the log.
func TestRecordAuditSkipsIngestAndAnonymousActors(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	require.NoError(t, env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		require.NoError(t, events.RecordAudit(ctx, pub, auditEntry(events.ActorIngest)))
		return events.RecordAudit(ctx, pub, auditEntry(""))
	}))
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameAuditEntry))
}

// A rolled-back mutation leaves no Audit entry.
func TestRecordAuditRollsBackWithTheMutation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	boom := errors.New("mutation failed")

	err := env.Bus.WithinTx(ctx, func(_ *ent.Client, pub events.Publisher) error {
		require.NoError(t, events.RecordAudit(ctx, pub, auditEntry(events.ActorUser)))
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Empty(t, env.QueuedEnvelopes(t, events.NameAuditEntry))
}
