package scopedclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/tag"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

var johnActor = events.Actor{Kind: events.ActorUser, ID: strconv.Itoa(fixtures.OwnerJohnID), Name: "John"}

// auditEntries decodes the Audit entries published so far in this test's transaction.
func auditEntries(t *testing.T, env *testhelper.TestEnv) []*events.AuditEntry {
	t.Helper()
	return lo.Map(env.OutboxEvents(t, events.NameAuditEntry), func(ev events.DomainEvent, _ int) *events.AuditEntry {
		entry, ok := ev.(*events.AuditEntry)
		require.True(t, ok)
		return entry
	})
}

func diffJSON(t *testing.T, e *events.AuditEntry) string {
	t.Helper()
	b, err := json.Marshal(e.Diff)
	require.NoError(t, err)
	return string(b)
}

// A Tag written under a User actor commits together with its Audit entry, in a
// transaction the wrapper opens itself; the entry carries a before/after diff.
func TestAuditedTagWritesRecordActorActionAndDiff(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.Bus.Act(env.DB.Scoped(acme), johnActor)

	// test writes: the scoped wrapper is the subject
	created, err := s.Tag().Create().SetName("audited").Save(ctx)
	require.NoError(t, err)
	require.NoError(t, s.Tag().UpdateOneID(created.ID).SetName("renamed").Exec(ctx))
	require.NoError(t, s.Tag().DeleteOneID(created.ID).Exec(ctx))

	got := auditEntries(t, env)
	require.Len(t, got, 3)
	for _, e := range got {
		assert.Equal(t, johnActor, e.Actor)
		assert.Equal(t, acme, e.WorkspaceID)
		assert.Equal(t, "tag", e.TargetType)
		assert.Equal(t, strconv.FormatInt(created.ID, 10), e.TargetID)
	}
	assert.Equal(t, "tag.create", got[0].Action)
	assert.Equal(t, "audited", got[0].TargetName)
	assert.JSONEq(t, `{"name":{"to":"audited"}}`, diffJSON(t, got[0]))
	assert.Equal(t, "tag.update", got[1].Action)
	assert.Equal(t, "renamed", got[1].TargetName)
	assert.JSONEq(t, `{"name":{"from":"audited","to":"renamed"}}`, diffJSON(t, got[1]))
	assert.Equal(t, "tag.delete", got[2].Action)
	assert.Equal(t, "renamed", got[2].TargetName, "the name is snapshotted before the row is gone")
	assert.JSONEq(t, `{"name":{"from":"renamed"}}`, diffJSON(t, got[2]))
}

func TestAuditedBulkWritesRecordOneEntryPerRow(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.Bus.Act(env.DB.Scoped(acme), johnActor)

	// test writes: the scoped wrapper is the subject
	_, err := s.Tag().CreateBulk(s.Tag().Create().SetName("bulk-a"), s.Tag().Create().SetName("bulk-b")).Save(ctx)
	require.NoError(t, err)
	n, err := s.Tag().Update().Where(tag.Name("bulk-a")).SetName("bulk-c").Save(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	n, err = s.Tag().Delete().Where(tag.NameHasPrefix("bulk-")).Exec(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	actions := lo.Map(auditEntries(t, env), func(e *events.AuditEntry, _ int) string { return e.Action + ":" + e.TargetName })
	assert.ElementsMatch(t, []string{
		"tag.create:bulk-a", "tag.create:bulk-b", "tag.update:bulk-c", "tag.delete:bulk-c", "tag.delete:bulk-b",
	}, actions)
}

// A rolled-back mutation leaves no entry; a joined scope never opens a transaction of its
// own, so the outer transaction decides.
func TestAuditedWriteRollsBackWithItsTransaction(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.Bus.Act(env.DB.Scoped(acme), johnActor)
	boom := errors.New("boom")

	// A failed audited write (unique name) leaves no entry.
	// test writes: the scoped wrapper is the subject
	_, err := s.Tag().Create().SetName("vip").Save(ctx)
	require.Error(t, err)
	assert.Empty(t, auditEntries(t, env))

	// An outer transaction that rolls back takes the entry with it.
	err = env.Bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		// test writes: the scoped wrapper is the subject
		require.NoError(t, ts.Tag().Create().SetName("rolled-back").Exec(ctx))
		return boom
	})
	require.ErrorIs(t, err, boom)
	assert.Empty(t, auditEntries(t, env))
	exists, err := env.DB.Scoped(acme).Tag().Query().Where(tag.Name("rolled-back")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, exists)

	// One that commits keeps the mutation and the entry, under the scope's actor.
	require.NoError(t, env.Bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		// test writes: the scoped wrapper is the subject
		return ts.Tag().Create().SetName("kept").Exec(ctx)
	}))
	got := auditEntries(t, env)
	require.Len(t, got, 1, "WithinScopedTx keeps the actor of the scope")
	assert.Equal(t, johnActor, got[0].Actor)
	assert.Equal(t, "kept", got[0].TargetName)
}

type countingOpener struct{ calls int }

func (o *countingOpener) WithinAuditTx(context.Context, *ent.Scoped, func(*ent.Scoped) error) error {
	o.calls++
	return errors.New("a joined scope must not open a transaction")
}

type recordingPublisher struct{ changes []ent.AuditChange }

func (p *recordingPublisher) PublishAudit(_ context.Context, _ int64, c ent.AuditChange) error {
	p.changes = append(p.changes, c)
	return nil
}

// "Joined, not nested": a scope bound to a transaction publishes in it and never asks its
// opener for another one. (Under go-txdb a nested transaction would be a savepoint and
// rollback would still pass, so the opener is a stub that counts.)
func TestAuditedWriteJoinsTheTransactionOfItsScope(t *testing.T) {
	env := testhelper.Setup(t)
	opener, pub := &countingOpener{}, &recordingPublisher{}
	s := env.DB.Scoped(acme).As(johnActor, opener).InTx(env.DB, pub)

	// test writes: the scoped wrapper is the subject
	require.NoError(t, s.Tag().Create().SetName("joined").Exec(t.Context()))

	assert.Zero(t, opener.calls)
	require.Len(t, pub.changes, 1)
	assert.Equal(t, "tag.create", pub.changes[0].Action)
	assert.Equal(t, johnActor, pub.changes[0].Actor)
}

func TestAuditedActorWithoutAnOpenerIsAnError(t *testing.T) {
	env := testhelper.Setup(t)
	s := env.DB.Scoped(acme).As(johnActor, nil)

	// test writes: the scoped wrapper is the subject
	err := s.Tag().Create().SetName("no-opener").Exec(t.Context())
	require.ErrorIs(t, err, ent.ErrAuditUnavailable)
}

// Ingest and actor-less scopes take the plain path: no transaction, no entry. The same
// write under a system actor is recorded and labelled.
func TestIngestAndAnonymousScopesAreNeverAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	// test writes: the scoped wrapper is the subject
	require.NoError(t, events.Ingest(env.DB.Scoped(acme)).Tag().Create().SetName("ingested").Exec(ctx))
	// test writes: the scoped wrapper is the subject
	require.NoError(t, env.DB.Scoped(acme).Tag().Create().SetName("anonymous").Exec(ctx))
	// test writes: the scoped wrapper is the subject
	require.NoError(t, env.Bus.Act(env.DB.Scoped(acme), events.Actor{Kind: events.ActorSystem}).Tag().Create().SetName("by-system").Exec(ctx))

	got := auditEntries(t, env)
	require.Len(t, got, 1)
	assert.Equal(t, events.ActorSystem, got[0].Actor.Kind)
	assert.Equal(t, "by-system", got[0].TargetName)
}

// An upsert of an audited entity can only be DO NOTHING: an insert is audited, a conflict
// writes and records nothing.
func TestAuditedUpsertRecordsOnlyTheInsert(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.Bus.Act(env.DB.Scoped(acme), johnActor)
	upsert := func(name string) error {
		// test writes: the scoped wrapper is the subject
		return s.Tag().Create().SetName(name).OnConflictColumns(tag.FieldName, tag.FieldWorkspaceID).DoNothing().Exec(ctx)
	}

	require.NoError(t, upsert("upserted"))
	require.ErrorContains(t, upsert("upserted"), "no rows")
	require.ErrorContains(t, upsert("vip"), "no rows")
	assert.Len(t, auditEntries(t, env), 1)

	// test writes: the scoped wrapper is the subject
	err := s.Tag().Create().SetName("x").OnConflictColumns(tag.FieldName, tag.FieldWorkspaceID).Ignore().Exec(ctx)
	require.ErrorIs(t, err, ent.ErrAuditUpsert)
}

// A sensitive field records only that it changed, never its value; this is the one
// function every audited entity's diff goes through.
func TestAuditDiffRedactsSensitiveFields(t *testing.T) {
	sensitive := map[string]bool{"secret": true}

	created := ent.AuditDiff(nil, map[string]any{"name": "n", "secret": "s3"}, sensitive)
	updated := ent.AuditDiff(map[string]any{"name": "n", "secret": "old"}, map[string]any{"name": "n", "secret": "new"}, sensitive)
	deleted := ent.AuditDiff(map[string]any{"name": "n", "secret": "old"}, nil, sensitive)

	assert.Equal(t, map[string]any{"name": map[string]any{"to": "n"}, "secret": "changed"}, created)
	assert.Equal(t, map[string]any{"secret": "changed"}, updated, "an unchanged field is absent")
	assert.Equal(t, map[string]any{"name": map[string]any{"from": "n"}, "secret": "changed"}, deleted)
	for _, d := range []map[string]any{created, updated, deleted} {
		b, err := json.Marshal(d)
		require.NoError(t, err)
		assert.False(t, strings.Contains(string(b), "s3") || strings.Contains(string(b), "old") || strings.Contains(string(b), "new"))
	}
}

// makeRoomForPlanted clears a singleton entity's own row first: the scope without an actor writes
// no entry, so the planted copy takes its place unaudited.
func makeRoomForPlanted(t *testing.T, env *testhelper.TestEnv, e ent.ScopedEntity, ids []int64) {
	t.Helper()
	if singletonEntities[e.Name] {
		require.NoError(t, e.Delete(t.Context(), env.DB.Scoped(acme), ids[0]), e.Name)
	}
}

// Every entity that opts in emits on create, update and delete (so a new annotated entity
// cannot silently miss a wrapper). The registry is generated from the schema.
func TestEveryAuditedEntityEmitsOnCreateUpdateAndDelete(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.Bus.Act(env.DB.Scoped(acme), johnActor)

	auditedEntities := lo.Filter(ent.ScopedEntities(), func(e ent.ScopedEntity, _ int) bool { return e.Audited })
	for _, e := range auditedEntities {
		ids, err := e.IDs(ctx, env.DB, acme)
		require.NoError(t, err)
		require.NotEmpty(t, ids, e.Name)
		foreign, err := e.IDs(ctx, env.DB, globex)
		require.NoError(t, err)
		require.NotEmpty(t, foreign, e.Name)
		// The scope without an actor writes no entry, so a singleton makes room for the planted row
		// unaudited.
		makeRoomForPlanted(t, env, e, ids)
		before := len(auditEntries(t, env))

		// Create a row by planting a copy of Globex's (no unique clash with Acme's rows),
		// pointing its references at Acme rows.
		remap := func(entity string, _ int64) int64 {
			for _, target := range ent.ScopedEntities() {
				if target.Name == entity {
					refs, err := target.IDs(ctx, env.DB, acme)
					require.NoError(t, err)
					return refs[0]
				}
			}
			return 0
		}
		_, err = e.Replant(ctx, s, foreign[0], remap)
		require.NoError(t, err, e.Name)
		// The planted row has the newest id; a fixture row may have rows that stand on it.
		ids, err = e.IDs(ctx, env.DB, acme)
		require.NoError(t, err)
		planted := slices.Max(ids)
		require.NoError(t, e.Touch(ctx, s, planted), e.Name)
		require.NoError(t, e.Delete(ctx, s, planted), e.Name)

		got := auditEntries(t, env)[before:]
		require.Len(t, got, 3, e.Name)
		for i, verb := range []string{"create", "update", "delete"} {
			assert.True(t, strings.HasSuffix(got[i].Action, "."+verb), "%s: %s", e.Name, got[i].Action)
			assert.Equal(t, johnActor, got[i].Actor, e.Name)
		}
	}
	assert.NotEmpty(t, auditedEntities, "Tag is audited")
}
