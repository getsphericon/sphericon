package tags_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/app/entclient"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/tags"
)

// closedClient is an ent client, and a bus, whose database is gone: every query fails.
func closedClient(t *testing.T) (*ent.Client, *events.Bus) {
	t.Helper()
	sqlDB, err := sql.Open("pgx", "postgres://closed.invalid/none")
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return entclient.New(sqlDB), events.New(sqlDB, entclient.NewTx)
}

// A storage failure surfaces as the underlying error: it is never mistaken for a
// missing contact, an invalid name, or an empty catalogue.
func TestStorageFailuresSurfaceAsErrors(t *testing.T) {
	client, bus := closedClient(t)
	m, c := tags.New(), contacts.New(bus)
	ctx := context.Background()
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
