package entclient_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An update moves the revision unless it writes Bookkeeping columns alone (ADR 0119): a delivery
// that records a failure leaves an endpoint's editor able to save, and a write of anything a person
// edits, alone or with them, makes it stale.
func TestAnUpdateOfBookkeepingColumnsAloneLeavesTheRevision(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	id := int64(fixtures.WebhookCodebasicsID)
	revision := func() int64 {
		t.Helper()
		return env.DB.WebhookEndpoint.GetX(ctx, id).Revision
	}
	before := revision()

	env.DB.WebhookEndpoint.UpdateOneID(id).AddConsecutiveFailures(1).SetLastFailureAt(time.Now()).ExecX(ctx)
	assert.Equal(t, before, revision(), "a failed delivery's record alone")

	env.DB.WebhookEndpoint.UpdateOneID(id).ClearConsecutiveFailures().ClearLastFailureAt().ExecX(ctx)
	assert.Equal(t, before, revision(), "a successful delivery's clear alone")

	env.DB.WebhookEndpoint.UpdateOneID(id).SetURL("https://hooks.example.com/renamed").ExecX(ctx)
	assert.Equal(t, before+1, revision(), "a column an editor sends")

	env.DB.WebhookEndpoint.UpdateOneID(id).AddHealthGeneration(1).SetLastFailureAt(time.Now()).ExecX(ctx)
	assert.Equal(t, before+2, revision(), "an added column an editor's write moves, with a bookkeeping one")
}

// An update that writes no column but updated_at (a Contact's Tag change touches the Contact) is a
// write of what an editor shows: it moves the revision.
func TestAnUpdateOfNoColumnMovesTheRevision(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	before := env.DB.Contact.GetX(ctx, fixtures.ContactAliceID).Revision

	env.DB.Contact.UpdateOneID(fixtures.ContactAliceID).ExecX(ctx)

	assert.Equal(t, before+1, env.DB.Contact.GetX(ctx, fixtures.ContactAliceID).Revision)
}
