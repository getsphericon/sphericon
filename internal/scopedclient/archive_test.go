package scopedclient_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/scopedtest"
	"github.com/getsphericon/sphericon/ent/scopedtest/plan"
	"github.com/getsphericon/sphericon/internal/cleanup"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The ArchiveMixin (ADR 0100) as the templates generate it, seen on the test-only client
// ent/scopedtest: its Plan is archivable, audited and a Versioned definition. No production entity
// takes the mixin yet, so the client has no fixtures; each test makes its tables and rows in a
// database of its own.

// probe is the test-only client over an empty database, with two Workspaces and a Plan in each.
type probe struct {
	c             *scopedtest.Client
	ws, other     int64
	plan, foreign int64
	changes       *[]scopedtest.AuditChange
	opener        probeOpener
}

func newProbe(t *testing.T) probe {
	t.Helper()
	ctx := t.Context()
	sqlDB, err := sql.Open("pgx", testhelper.EmptyDBURL(t))
	require.NoError(t, err)
	t.Cleanup(func() { cleanup.Close(sqlDB, "scopedtest db") })
	c := scopedtest.NewClient(scopedtest.Driver(entsql.OpenDB(dialect.Postgres, sqlDB)))
	require.NoError(t, c.Schema.Create(ctx))

	p := probe{c: c, ws: 1, other: 2, changes: &[]scopedtest.AuditChange{}}
	p.opener = probeOpener{c: c, changes: p.changes}
	for _, ws := range []int64{p.ws, p.other} {
		// test writes: the test-only client has no fixtures; its tenants are made here
		c.Workspace.Create().SetID(ws).ExecX(ctx)
	}
	// test writes: the test-only client has no fixtures; the Plan under test is made here
	kept, err := c.Scoped(p.ws).Plan().Create().SetName("kept").Save(ctx)
	require.NoError(t, err)
	// test writes: the test-only client has no fixtures; another Workspace's Plan is made here
	foreign, err := c.Scoped(p.other).Plan().Create().SetName("foreign").Save(ctx)
	require.NoError(t, err)
	p.plan, p.foreign = kept.ID, foreign.ID
	return p
}

func (p probe) scope() *scopedtest.Scoped { return p.c.Scoped(p.ws) }

func (p probe) listed(t *testing.T, q *scopedtest.PlanQuery) bool {
	t.Helper()
	ok, err := q.Where(plan.ID(p.plan)).Exist(t.Context())
	require.NoError(t, err)
	return ok
}

// probeOpener runs an audited write in a transaction of the test-only client and keeps the changes
// it publishes.
type probeOpener struct {
	c       *scopedtest.Client
	changes *[]scopedtest.AuditChange
}

func (o probeOpener) WithinAuditTx(ctx context.Context, s *scopedtest.Scoped, fn func(ts *scopedtest.Scoped) error) error {
	tx, err := o.c.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(s.InTx(tx.Client(), o)); err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func (o probeOpener) PublishAudit(_ context.Context, _ int64, change scopedtest.AuditChange) error {
	*o.changes = append(*o.changes, change)
	return nil
}

// An archivable entity has no Delete and no Discard on its scoped wrapper: it is archived and
// unarchived, and only Purge (Workspace erasure) removes it.
func TestArchivableEntityHasArchiveAndNoDelete(t *testing.T) {
	wrapper := reflect.TypeFor[*scopedtest.PlanScoped]()

	for _, absent := range []string{"Delete", "DeleteOneID", "Discard", "Undiscard", "Restore", "QueryWithDiscarded"} {
		_, has := wrapper.MethodByName(absent)
		assert.Falsef(t, has, "an archivable wrapper has no %s", absent)
	}
	for _, present := range []string{"Archive", "Unarchive", "QueryArchived", "QueryWithArchived", "Purge", "PurgeOneID"} {
		_, has := wrapper.MethodByName(present)
		assert.Truef(t, has, "an archivable wrapper has %s", present)
	}
}

// An archived row leaves list queries, is listed by the archived view, is still read and written
// by id, and comes back with Unarchive.
func TestArchiveHidesFromListsAndKeepsByIDReadsAndWrites(t *testing.T) {
	p := newProbe(t)
	ctx := t.Context()
	s := p.scope()

	require.NoError(t, s.Plan().Archive(ctx, p.plan))

	assert.False(t, p.listed(t, s.Plan().Query()), "a list query hides the archived row")
	assert.True(t, p.listed(t, s.Plan().QueryArchived()), "the archived view lists it")
	assert.True(t, p.listed(t, s.Plan().QueryWithArchived()))
	got, err := s.Plan().Get(ctx, p.plan)
	require.NoError(t, err)
	assert.NotNil(t, got.ArchivedAt, "a by-id read sees the archived row")
	renamed, err := s.Plan().UpdateOneID(p.plan).SetName("written while archived").Save(ctx)
	require.NoError(t, err, "the system still writes an archived row")
	assert.Equal(t, "written while archived", renamed.Name)

	require.NoError(t, s.Plan().Unarchive(ctx, p.plan))

	assert.True(t, p.listed(t, s.Plan().Query()))
	assert.False(t, p.listed(t, s.Plan().QueryArchived()))
}

// Archive applies only to a kept row its predicates match, never across Workspaces; Unarchive of a
// kept row changes nothing, and only a missing row is not found.
func TestArchiveIsFencedByPredicatesStateAndWorkspace(t *testing.T) {
	p := newProbe(t)
	ctx := t.Context()
	s := p.scope()

	fenced := s.Plan().Archive(ctx, p.plan, plan.Name("another name"))
	assert.True(t, scopedtest.IsNotFound(fenced), "a predicate the row does not match fences it")
	require.NoError(t, s.Plan().Unarchive(ctx, p.plan), "unarchiving a kept row changes nothing")
	assert.True(t, p.listed(t, s.Plan().Query()))

	require.NoError(t, s.Plan().Archive(ctx, p.plan))
	twice := s.Plan().Archive(ctx, p.plan)
	assert.True(t, scopedtest.IsNotFound(twice), "already archived")

	assert.True(t, scopedtest.IsNotFound(s.Plan().Archive(ctx, p.foreign)))
	assert.True(t, scopedtest.IsNotFound(s.Plan().Unarchive(ctx, p.foreign)))
	_, err := s.Plan().Get(ctx, p.foreign)
	assert.True(t, scopedtest.IsNotFound(err))
}

// The Audit log names the moves as the owner sees them: archive and unarchive (ADR 0100), and the
// registry lists both for the archivable kind.
func TestArchiveAndUnarchiveAreAudited(t *testing.T) {
	p := newProbe(t)
	ctx := t.Context()
	s := p.scope().As(scopedtest.Actor{Kind: scopedtest.ActorUser, ID: "1", Name: "John"}, p.opener)

	require.NoError(t, s.Plan().Archive(ctx, p.plan))
	require.NoError(t, s.Plan().Unarchive(ctx, p.plan))
	require.NoError(t, s.Plan().Unarchive(ctx, p.plan))

	actions := lo.Map(*p.changes, func(c scopedtest.AuditChange, _ int) string { return c.Action })
	assert.Equal(t, []string{"plan.archive", "plan.unarchive"}, actions, "unarchiving a kept row records nothing")
	assert.Contains(t, (*p.changes)[0].Diff, plan.FieldArchivedAt)

	registered, ok := lo.Find(scopedtest.ScopedEntities(), func(e scopedtest.ScopedEntity) bool { return e.Name == "Plan" })
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"plan.create", "plan.update", "plan.delete", "plan.archive", "plan.unarchive"}, registered.AuditActions)
}

// The storage half of an archivable Versioned definition archives through archived_at, reports it
// as Archived (never Discarded), and refuses to discard.
func TestAnArchivableVersionedStoreArchivesAndNeverDiscards(t *testing.T) {
	p := newProbe(t)
	ctx := t.Context()
	var store scopedtest.VersionedStore[scopedtest.Plan, scopedtest.PlanVersion] = p.scope().Plan().Pointers()

	require.NoError(t, store.Archive(ctx, p.plan))
	archived, err := store.Lock(ctx, p.plan)
	require.NoError(t, err)
	require.NoError(t, store.Unarchive(ctx, p.plan))
	kept, err := store.Lock(ctx, p.plan)
	require.NoError(t, err)

	assert.True(t, archived.Archived)
	assert.False(t, archived.Discarded)
	assert.False(t, kept.Archived)
	require.ErrorIs(t, store.Discard(ctx, p.plan), scopedtest.ErrNotDiscardable)
}
