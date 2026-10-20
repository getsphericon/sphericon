package scopedclient_test

import (
	"context"
	"errors"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acme, globex int64 = fixtures.AcmeID, fixtures.GlobexID

// singletonEntities hold at most one row per Workspace (a unique index on workspace_id). A test
// that plants a second row in Acme first removes Acme's own.
var singletonEntities = map[string]bool{"BrandKit": true}

// firstID returns the lowest id of the entity's rows in the Workspace, read through
// the raw client. A missing row means a fixture row is missing for the entity.
func firstID(ctx context.Context, t *testing.T, c *ent.Client, entities []ent.ScopedEntity, name string, ws int64) int64 {
	t.Helper()
	e, ok := lo.Find(entities, func(e ent.ScopedEntity) bool { return e.Name == name })
	require.True(t, ok, "no scoped entity %s", name)
	ids, err := e.IDs(ctx, c, ws)
	require.NoError(t, err)
	require.NotEmpty(t, ids, "add a %s fixture row to workspace %d", name, ws)
	return ids[0]
}

// replantInAcme re-creates the Globex row in Acme with every reference pointed at an
// Acme row. A unique index (e.g. one run per automation and contact) can reject a
// pairing, so it tries the next Acme rows, each attempt in its own savepoint.
func replantInAcme(ctx context.Context, t *testing.T, c *ent.Client, entities []ent.ScopedEntity, e ent.ScopedEntity, from int64) (int64, error) {
	t.Helper()
	// Read before the transaction opens: the test connection serves one query at a time.
	acmeIDs := map[string][]int64{}
	for _, te := range entities {
		ids, err := te.IDs(ctx, c, acme)
		require.NoError(t, err)
		acmeIDs[te.Name] = ids
	}
	for attempt := 0; ; attempt++ {
		exhausted := false
		remap := func(target string, _ int64) int64 {
			ids := acmeIDs[target]
			require.NotEmpty(t, ids, "add a %s fixture row to workspace %d", target, acme)
			exhausted = attempt >= len(ids)
			return ids[attempt%len(ids)]
		}
		tx, err := c.Tx(ctx)
		require.NoError(t, err)
		ws, err := e.Replant(ctx, tx.Client().Scoped(acme), from, remap)
		if err == nil {
			require.NoError(t, tx.Commit())
			return ws, nil
		}
		require.NoError(t, tx.Rollback())
		if !ent.IsConstraintError(err) || exhausted {
			return 0, err
		}
	}
}

// clearSingleton removes a singleton entity's own row, so the re-created foreign copy can take its place.
func clearSingleton(t *testing.T, ctx context.Context, scoped *ent.Scoped, e ent.ScopedEntity, own []int64) {
	t.Helper()
	if singletonEntities[e.Name] {
		require.NoError(t, e.Delete(ctx, scoped, own[0]))
	}
}

// TestScopedClientIsolation runs over the GENERATED list of workspace-mixin entities,
// so a new entity is covered by adding the workspace mixin to its schema.
func TestScopedClientIsolation(t *testing.T) {
	entities := ent.ScopedEntities()
	require.NotEmpty(t, entities)

	for _, e := range entities {
		t.Run(e.Name, func(t *testing.T) {
			env := testhelper.Setup(t)
			ctx := t.Context()
			scoped := env.DB.Scoped(acme)

			own, err := e.IDs(ctx, env.DB, acme)
			require.NoError(t, err)
			foreign, err := e.IDs(ctx, env.DB, globex)
			require.NoError(t, err)
			require.NotEmpty(t, own, "add a %s fixture row to Acme", e.Name)
			require.NotEmpty(t, foreign, "add a %s fixture row to Globex", e.Name)

			t.Run("own row is reachable", func(t *testing.T) {
				require.NoError(t, e.Get(ctx, scoped, own[0]))
				require.NoError(t, e.Touch(ctx, scoped, own[0]))
				n, err := e.BulkTouch(ctx, scoped, own[0])
				require.NoError(t, err)
				assert.Equal(t, 1, n)
			})

			t.Run("foreign row reads as not found and cannot be changed", func(t *testing.T) {
				id := foreign[0]
				assert.True(t, ent.IsNotFound(e.Get(ctx, scoped, id)), "Get")
				assert.True(t, ent.IsNotFound(e.Touch(ctx, scoped, id)), "UpdateOneID")
				n, err := e.BulkTouch(ctx, scoped, id)
				require.NoError(t, err)
				assert.Zero(t, n, "Update().Where")
				assert.True(t, ent.IsNotFound(e.Delete(ctx, scoped, id)), "DeleteOneID")
				n, err = e.BulkDelete(ctx, scoped, id)
				require.NoError(t, err)
				assert.Zero(t, n, "Delete().Where")

				after, err := e.IDs(ctx, env.DB, globex)
				require.NoError(t, err)
				assert.Equal(t, foreign, after, "the foreign rows are untouched")
			})

			t.Run("a foreign reference is refused", func(t *testing.T) {
				for _, ref := range e.Refs {
					target := firstID(ctx, t, env.DB, entities, ref.Target, globex)
					err := ref.Set(ctx, scoped, own[0], target)
					require.ErrorIs(t, err, ent.ErrNotInWorkspace, "ref %s -> %s", ref.Name, ref.Target)

					// The same reference to an own row is accepted.
					ownTarget := firstID(ctx, t, env.DB, entities, ref.Target, acme)
					assert.NotErrorIs(t, ref.Set(ctx, scoped, own[0], ownTarget), ent.ErrNotInWorkspace, "own ref %s -> %s", ref.Name, ref.Target)
				}
			})

			t.Run("create sets the workspace and refuses foreign references", func(t *testing.T) {
				// The foreign fixture row is re-created without its workspace_id:
				// the new row lands in Acme only because the scoped create set it.
				clearSingleton(t, ctx, scoped, e, own)
				ws, err := replantInAcme(ctx, t, env.DB, entities, e, foreign[0])
				require.NoError(t, err)
				assert.Equal(t, acme, ws)

				referenced := false
				_, err = e.Replant(ctx, scoped, foreign[0], func(_ string, id int64) int64 {
					referenced = true
					return id // keeps pointing at the Globex row
				})
				refused := !referenced || errors.Is(err, ent.ErrNotInWorkspace)
				assert.True(t, refused, "a reference to a foreign row is refused: %v", err)
			})
		})
	}
}
