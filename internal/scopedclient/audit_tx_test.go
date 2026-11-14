package scopedclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var errAfterWrite = errors.New("the transaction fails after the write")

// failAfterWrite opens the bus's transaction for an audited write and fails it once the write and
// its entry are done, so the transaction rolls back.
type failAfterWrite struct{ bus *events.Bus }

func (o failAfterWrite) WithinAuditTx(ctx context.Context, s *ent.Scoped, fn func(*ent.Scoped) error) error {
	return o.bus.WithinAuditTx(ctx, s, func(ts *ent.Scoped) error {
		if err := fn(ts); err != nil {
			return err
		}
		return errAfterWrite
	})
}

// An audited write through a scope bound to no transaction runs in the one its opener opens, on
// that transaction's client: when the transaction fails after the write, neither the row nor an
// entry is left, for every write the registry makes of every audited entity. Under Setup every
// connection rides the test's one transaction, and a write that escaped to the scope's own client
// would be rolled back with the audit transaction's savepoint, so the test commits on a database of
// its own.
func TestAnAuditedWriteRunsInTheTransactionItsOpenerOpens(t *testing.T) {
	ctx := t.Context()
	sqlDB := testhelper.RealDB(t)
	c := testhelper.Clients(t).New(sqlDB)
	bus := testhelper.BusOn(t, sqlDB)
	failing := c.Scoped(acme).As(johnActor, failAfterWrite{bus: bus})
	committing := bus.Act(c.Scoped(acme), johnActor)
	entries := func() int { return len(testhelper.QueuedEnvelopesOn(t, sqlDB, events.NameAuditEntry)) }

	entities := ent.ScopedEntities()
	audited := lo.Filter(entities, func(e *ent.ScopedEntity, _ int) bool { return len(e.AuditActions) > 0 })
	require.NotEmpty(t, audited)
	for _, e := range audited {
		t.Run(e.Name, func(t *testing.T) {
			ids, err := e.IDs(ctx, c, acme)
			require.NoError(t, err)
			foreign, err := e.IDs(ctx, c, globex)
			require.NoError(t, err)
			require.NotEmpty(t, foreign, "add a %s fixture row to Globex", e.Name)
			// A singleton makes room for the planted row.
			clearSingleton(ctx, t, committing, e, ids)
			ids, err = e.IDs(ctx, c, acme)
			require.NoError(t, err)
			remap := func(entity string, _ int64) int64 { return firstID(ctx, t, c, entities, entity, acme) }
			recorded := entries()

			_, err = e.Replant(ctx, failing, foreign[0], remap)
			require.ErrorIs(t, err, errAfterWrite, "create")
			after, err := e.IDs(ctx, c, acme)
			require.NoError(t, err)
			assert.Equal(t, ids, after, "a rolled-back create leaves no row")

			_, err = e.Replant(ctx, committing, foreign[0], remap)
			require.NoError(t, err)
			planted, err := e.IDs(ctx, c, acme)
			require.NoError(t, err)
			id := slices.Max(planted)
			// A row is compared by its JSON: the entity holds its client's config, which no two reads share.
			read := func() string {
				row, err := e.Get(ctx, c.Scoped(acme), id)
				require.NoError(t, err)
				b, err := json.Marshal(row)
				require.NoError(t, err)
				return string(b)
			}
			row := read()
			recorded = entries()

			for name, write := range map[string]func() error{
				"update by id": func() error { return e.Touch(ctx, failing, id) },
				"update":       func() error { _, err := e.BulkTouch(ctx, failing, id); return err },
				"delete by id": func() error { return e.Delete(ctx, failing, id) },
				"delete":       func() error { _, err := e.BulkDelete(ctx, failing, id); return err },
			} {
				require.ErrorIs(t, write(), errAfterWrite, name)
				assert.JSONEq(t, row, read(), "a rolled-back %s leaves the row as it was", name)
			}
			assert.Equal(t, recorded, entries(), "a rolled-back write leaves no entry")
		})
	}
}
