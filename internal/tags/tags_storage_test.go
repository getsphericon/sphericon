package tags_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/tags"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closedClient is an ent client whose database is gone: every query fails.
func closedClient(t *testing.T) *ent.Client {
	t.Helper()
	sqlDB, err := sql.Open("pgx", "postgres://closed.invalid/none")
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db.NewEntClient(sqlDB)
}

// A storage failure surfaces as the underlying error: it is never mistaken for a
// missing contact, an invalid name, or an empty catalogue.
func TestStorageFailuresSurfaceAsErrors(t *testing.T) {
	m := tags.New()
	ctx := context.Background()
	wsID := closedClient(t).Scoped(fixtures.AcmeID)

	_, err := m.List(ctx, wsID)
	require.Error(t, err)

	_, err = m.ForContact(ctx, wsID, fixtures.ContactAliceID)
	require.Error(t, err)
	require.NotErrorIs(t, err, tags.ErrContactNotFound)

	_, err = m.Apply(ctx, wsID, fixtures.ContactAliceID, "vip")
	require.Error(t, err)
	require.NotErrorIs(t, err, tags.ErrContactNotFound)

	err = m.Remove(ctx, wsID, fixtures.ContactAliceID, "vip")
	require.Error(t, err)
	assert.NotErrorIs(t, err, tags.ErrContactNotFound)
}

// Applying to a contact of another workspace is refused inside the transaction and
// rolls back: the tag is not auto-created.
func TestApplyToAnotherWorkspacesContactCreatesNoTag(t *testing.T) {
	env := testhelper.Setup(t)
	m := tags.New()
	ctx := context.Background()
	otherWsID := env.DB.Scoped(fixtures.GlobexID)

	_, err := m.Apply(ctx, otherWsID, fixtures.ContactAliceID, "sneaky")
	require.ErrorIs(t, err, tags.ErrContactNotFound)

	got, err := m.List(ctx, otherWsID)
	require.NoError(t, err)
	for _, tg := range got {
		assert.NotEqual(t, "sneaky", tg.Name)
	}
	err = m.Remove(ctx, otherWsID, fixtures.ContactAliceID, fixtures.TagVipName)
	assert.ErrorIs(t, err, tags.ErrContactNotFound)
}
