package tags_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/domain/contacts/confirmations"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/domain/contacts/engagementtracking"
	"github.com/getsphericon/sphericon/internal/domain/contacts/tags"
	"github.com/getsphericon/sphericon/internal/domain/contacts/unsubscribes"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// closedClient is an ent client, and a bus, whose database is gone: every query fails.
func closedClient(t *testing.T) (*ent.Client, *events.Bus) {
	t.Helper()
	sqlDB := testhelper.ClosedDB(t)
	return entclient.Core.New(sqlDB), events.New(sqlDB, entclient.Core.NewTx, time.Now)
}

// A storage failure surfaces as the underlying error: it is never mistaken for a
// missing contact, an invalid name, or an empty catalogue.
func TestStorageFailuresSurfaceAsErrors(t *testing.T) {
	client, bus := closedClient(t)
	m := tags.New()
	c := contacts.New(bus, m, confirmations.New(bus, unsubscribes.New(), time.Now), engagementtracking.New(bus, time.Now), nil, time.Now)
	ctx := t.Context()
	wsID := client.Scoped(fixtures.AcmeID)

	_, err := m.List(ctx, wsID)
	require.Error(t, err)

	_, err = m.ForContact(ctx, wsID, fixtures.ContactAliceID)
	require.Error(t, err)
	require.NotErrorIs(t, err, tags.ErrContactNotFound)

	_, err = c.ApplyTag(ctx, wsID, fixtures.ContactAliceID, "vip")
	require.Error(t, err)
	require.NotErrorIs(t, err, tags.ErrContactNotFound)

	err = c.RemoveTag(ctx, wsID, fixtures.ContactAliceID, "vip")
	require.Error(t, err)
	assert.NotErrorIs(t, err, tags.ErrContactNotFound)
}
