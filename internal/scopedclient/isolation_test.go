package scopedclient_test

import (
	"context"
	"errors"
	"testing"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const acme, globex int64 = fixtures.AcmeID, fixtures.GlobexID

// firstID returns the lowest id of the entity's rows in the Workspace, read through
// the raw client. A missing row means a fixture row is missing for the entity.
func firstID(ctx context.Context, t *testing.T, c *ent.Client, entities []*ent.ScopedEntity, name string, ws int64) int64 {
	t.Helper()
	e, ok := lo.Find(entities, func(e *ent.ScopedEntity) bool { return e.Name == name })
	require.True(t, ok, "no scoped entity %s", name)
	ids, err := e.IDs(ctx, c, ws)
	require.NoError(t, err)
	require.NotEmpty(t, ids, "add a %s fixture row to workspace %d", name, ws)
	return ids[0]
}

// inAcme runs write on Acme's scope in a transaction of the bus, as production makes every write
// (a write a hook watches, such as a Contact's, is refused anywhere else), and returns its error;
// the transaction rolls back on one.
func inAcme(ctx context.Context, env *testhelper.TestEnv, write func(scoped *ent.Scoped) error) error {
	return env.Bus.WithinTx(ctx, func(tx *ent.Client, _ events.Publisher) error {
		return write(tx.Scoped(acme))
	})
}

// replantInAcme re-creates the Globex row in Acme with every reference pointed at an
// Acme row. A unique index (e.g. one run per automation and contact) can reject a
// pairing, so it tries the next Acme rows, each attempt in a transaction of its own.
func replantInAcme(ctx context.Context, t *testing.T, env *testhelper.TestEnv, entities []*ent.ScopedEntity, e *ent.ScopedEntity, from int64) (int64, error) {
	t.Helper()
	acmeIDs := map[string][]int64{}
	for _, te := range entities {
		ids, err := te.IDs(ctx, env.DB, acme)
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
		var ws int64
		err := inAcme(ctx, env, func(scoped *ent.Scoped) (err error) {
			ws, err = e.Replant(ctx, scoped, from, remap)
			return err
		})
		if err == nil {
			return ws, nil
		}
		if !ent.IsConstraintError(err) || exhausted {
			return 0, err
		}
	}
}

// clearSingleton removes a singleton entity's own row (a Workspace holds at most one), so the
// re-created foreign copy can take its place.
func clearSingleton(ctx context.Context, t *testing.T, scoped *ent.Scoped, e *ent.ScopedEntity, own []int64) {
	t.Helper()
	if e.Singleton {
		require.NoError(t, e.Delete(ctx, scoped, own[0]), e.Name)
	}
}

// replantMakingRoom is replantInAcme, for an entity of one row per row it references (an
// Integration's provider state) too: when every Acme row it could reference has its own, those go to
// make room, and the row is re-created again.
func replantMakingRoom(ctx context.Context, t *testing.T, env *testhelper.TestEnv, entities []*ent.ScopedEntity, e *ent.ScopedEntity, own []int64, from int64) (int64, error) {
	t.Helper()
	ws, err := replantInAcme(ctx, t, env, entities, e, from)
	if !ent.IsConstraintError(err) {
		return ws, err
	}
	if err := inAcme(ctx, env, func(scoped *ent.Scoped) error {
		for _, id := range own {
			if err := e.Delete(ctx, scoped, id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return 0, err
	}
	return replantInAcme(ctx, t, env, entities, e, from)
}

// names lists the schema names of the entities.
func names(entities []*ent.ScopedEntity) []string {
	return lo.Map(entities, func(e *ent.ScopedEntity, _ int) string { return e.Name })
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
			read := env.DB.Scoped(acme)

			own, err := e.IDs(ctx, env.DB, acme)
			require.NoError(t, err)
			foreign, err := e.IDs(ctx, env.DB, globex)
			require.NoError(t, err)
			require.NotEmpty(t, own, "add a %s fixture row to Acme", e.Name)
			require.NotEmpty(t, foreign, "add a %s fixture row to Globex", e.Name)

			t.Run("own row is reachable", func(t *testing.T) {
				_, err := e.Get(ctx, read, own[0])
				require.NoError(t, err)
				require.NoError(t, inAcme(ctx, env, func(scoped *ent.Scoped) error { return e.Touch(ctx, scoped, own[0]) }))
				var n int
				require.NoError(t, inAcme(ctx, env, func(scoped *ent.Scoped) (err error) {
					n, err = e.BulkTouch(ctx, scoped, own[0])
					return err
				}))
				assert.Equal(t, 1, n)
			})

			t.Run("foreign row reads as not found and cannot be changed", func(t *testing.T) {
				id := foreign[0]
				_, err := e.Get(ctx, read, id)
				assert.True(t, ent.IsNotFound(err), "Get")
				assert.True(t, ent.IsNotFound(inAcme(ctx, env, func(scoped *ent.Scoped) error { return e.Touch(ctx, scoped, id) })), "UpdateOneID")
				var n int
				require.NoError(t, inAcme(ctx, env, func(scoped *ent.Scoped) (err error) {
					n, err = e.BulkTouch(ctx, scoped, id)
					return err
				}))
				assert.Zero(t, n, "Update().Where")
				assert.True(t, ent.IsNotFound(inAcme(ctx, env, func(scoped *ent.Scoped) error { return e.Delete(ctx, scoped, id) })), "DeleteOneID")
				require.NoError(t, inAcme(ctx, env, func(scoped *ent.Scoped) (err error) {
					n, err = e.BulkDelete(ctx, scoped, id)
					return err
				}))
				assert.Zero(t, n, "Delete().Where")

				after, err := e.IDs(ctx, env.DB, globex)
				require.NoError(t, err)
				assert.Equal(t, foreign, after, "the foreign rows are untouched")
			})

			t.Run("a foreign reference is refused", func(t *testing.T) {
				for _, ref := range e.Refs {
					target := firstID(ctx, t, env.DB, entities, ref.Target, globex)
					err := inAcme(ctx, env, func(scoped *ent.Scoped) error { return ref.Set(ctx, scoped, own[0], target) })
					require.ErrorIs(t, err, ent.ErrNotInWorkspace, "ref %s -> %s", ref.Name, ref.Target)

					// The same reference to an own row is accepted.
					ownTarget := firstID(ctx, t, env.DB, entities, ref.Target, acme)
					err = inAcme(ctx, env, func(scoped *ent.Scoped) error { return ref.Set(ctx, scoped, own[0], ownTarget) })
					assert.NotErrorIs(t, err, ent.ErrNotInWorkspace, "own ref %s -> %s", ref.Name, ref.Target)
				}
			})

			t.Run("create sets the workspace and refuses foreign references", func(t *testing.T) {
				// The foreign fixture row is re-created without its workspace_id:
				// the new row lands in Acme only because the scoped create set it.
				require.NoError(t, inAcme(ctx, env, func(scoped *ent.Scoped) error {
					clearSingleton(ctx, t, scoped, e, own)
					return nil
				}))
				ws, err := replantMakingRoom(ctx, t, env, entities, e, own, foreign[0])
				require.NoError(t, err)
				assert.Equal(t, acme, ws)

				referenced := false
				err = inAcme(ctx, env, func(scoped *ent.Scoped) error {
					_, err := e.Replant(ctx, scoped, foreign[0], func(_ string, id int64) int64 {
						referenced = true
						return id // keeps pointing at the Globex row
					})
					return err
				})
				refused := !referenced || errors.Is(err, ent.ErrNotInWorkspace)
				assert.True(t, refused, "a reference to a foreign row is refused: %v", err)
			})
		})
	}
}
