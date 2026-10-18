package instance_test

import (
	"database/sql"
	"fmt"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/instance"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Workspaces cannot be deleted through the product yet and a fixture Workspace has rows in
// most tables, so the "oldest Workspace is gone" rule runs on a database of its own that
// holds only the two Workspaces it is about.
func TestIsOwnerPassesToTheNextOldestWorkspaceWhenTheOldestIsDeleted(t *testing.T) {
	ctx := t.Context()
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	_, err = db.Migrate(ctx, sqlDB)
	require.NoError(t, err)
	// No workspace-feed guard: this database has no feed reader.
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	mod := instance.New(client, instance.Environment{}, testHost, nil)

	for _, u := range []struct {
		id        int64
		workspace int64
	}{{1, 1}, {2, 2}} {
		require.NoError(t, client.User.Create().SetID(u.id).SetName("u").SetEmail(fmt.Sprintf("u%d@example.test", u.id)).Exec(ctx))
		require.NoError(t, client.Workspace.Create().SetID(u.workspace).SetName("w").SetSlug(fmt.Sprintf("w%d", u.workspace)).
			SetCollectKey(fmt.Sprintf("c%d", u.workspace)).SetIngestKey(fmt.Sprintf("i%d", u.workspace)).Exec(ctx))
		require.NoError(t, client.Scoped(u.workspace).Membership().Create().SetUserID(u.id).SetRole(membership.RoleOwner).Exec(ctx))
	}
	first, err := mod.IsOwner(ctx, 1)
	require.NoError(t, err)
	second, err := mod.IsOwner(ctx, 2)
	require.NoError(t, err)
	assert.True(t, first)
	assert.False(t, second)

	_, err = client.Scoped(1).Membership().Delete().Where(membership.UserID(1)).Exec(ctx)
	require.NoError(t, err)
	require.NoError(t, client.Workspace.DeleteOneID(1).Exec(ctx))

	first, err = mod.IsOwner(ctx, 1)
	require.NoError(t, err)
	second, err = mod.IsOwner(ctx, 2)
	require.NoError(t, err)
	assert.False(t, first)
	assert.True(t, second)
}
