package scopedclient_test

import (
	"reflect"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automation"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The ArchiveMixin (ADR 0100) as the templates generate it, seen on the Automation: archivable,
// audited and a Versioned definition.

const (
	keptAutomation    int64 = fixtures.AutomationWinBackID
	foreignAutomation int64 = fixtures.AutomationGlobexID
)

func automationListed(t *testing.T, q *ent.AutomationQuery) bool {
	t.Helper()
	ok, err := q.Where(automation.ID(keptAutomation)).Exist(t.Context())
	require.NoError(t, err)
	return ok
}

// An archivable entity has no Delete and no Discard on its scoped wrapper: it is archived and
// unarchived, and only Purge (Workspace erasure) removes it.
func TestArchivableEntityHasArchiveAndNoDelete(t *testing.T) {
	for _, wrapper := range []reflect.Type{
		reflect.TypeFor[*ent.BroadcastScoped](),
		reflect.TypeFor[*ent.AutomationScoped](),
		reflect.TypeFor[*ent.SegmentScoped](),
	} {
		for _, absent := range []string{"Delete", "DeleteOneID", "Discard", "Undiscard", "Restore", "QueryWithDiscarded"} {
			_, has := wrapper.MethodByName(absent)
			assert.Falsef(t, has, "%s has no %s", wrapper, absent)
		}
		for _, present := range []string{"Archive", "Unarchive", "QueryArchived", "QueryWithArchived", "Purge", "PurgeOneID"} {
			_, has := wrapper.MethodByName(present)
			assert.Truef(t, has, "%s has %s", wrapper, present)
		}
	}
}

// An archived row leaves list queries, is listed by the archived view, is still read and written
// by id, and comes back with Unarchive.
func TestArchiveHidesFromListsAndKeepsByIDReadsAndWrites(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(acme)

	require.NoError(t, s.Automation().Archive(ctx, keptAutomation))

	assert.False(t, automationListed(t, s.Automation().Query()), "a list query hides the archived row")
	assert.True(t, automationListed(t, s.Automation().QueryArchived()), "the archived view lists it")
	assert.True(t, automationListed(t, s.Automation().QueryWithArchived()))
	got, err := s.Automation().Get(ctx, keptAutomation)
	require.NoError(t, err)
	assert.NotNil(t, got.ArchivedAt, "a by-id read sees the archived row")
	renamed, err := s.Automation().UpdateOneID(keptAutomation).SetName("written while archived").Save(ctx)
	require.NoError(t, err, "the system still writes an archived row")
	assert.Equal(t, "written while archived", renamed.Name)

	require.NoError(t, s.Automation().Unarchive(ctx, keptAutomation))

	assert.True(t, automationListed(t, s.Automation().Query()))
	assert.False(t, automationListed(t, s.Automation().QueryArchived()))
}

// Archive applies only to a kept row its predicates match, never across Workspaces; Unarchive of a
// kept row changes nothing, and only a missing row is not found.
func TestArchiveIsFencedByPredicatesStateAndWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.DB.Scoped(acme)

	fenced := s.Automation().Archive(ctx, keptAutomation, automation.Name("another name"))
	assert.True(t, ent.IsNotFound(fenced), "a predicate the row does not match fences it")
	require.NoError(t, s.Automation().Unarchive(ctx, keptAutomation), "unarchiving a kept row changes nothing")
	assert.True(t, automationListed(t, s.Automation().Query()))

	require.NoError(t, s.Automation().Archive(ctx, keptAutomation))
	twice := s.Automation().Archive(ctx, keptAutomation)
	assert.True(t, ent.IsNotFound(twice), "already archived")

	assert.True(t, ent.IsNotFound(s.Automation().Archive(ctx, foreignAutomation)))
	assert.True(t, ent.IsNotFound(s.Automation().Unarchive(ctx, foreignAutomation)))
	_, err := s.Automation().Get(ctx, foreignAutomation)
	assert.True(t, ent.IsNotFound(err))
}

// The Audit log names the moves as the owner sees them: archive and unarchive (ADR 0100), and the
// registry lists both for the archivable kind.
func TestArchiveAndUnarchiveAreAudited(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	s := env.Bus.Act(env.DB.Scoped(acme), johnActor)

	require.NoError(t, s.Automation().Archive(ctx, keptAutomation))
	require.NoError(t, s.Automation().Unarchive(ctx, keptAutomation))
	require.NoError(t, s.Automation().Unarchive(ctx, keptAutomation))

	entries := auditEntries(t, env)
	actions := lo.Map(entries, func(e *events.AuditEntry, _ int) string { return e.Action })
	assert.Equal(t, []string{"automation.archive", "automation.unarchive"}, actions, "unarchiving a kept row records nothing")
	assert.Contains(t, entries[0].Diff, automation.FieldArchivedAt)

	registered, ok := lo.Find(ent.ScopedEntities(), func(e ent.ScopedEntity) bool { return e.Name == "Automation" })
	require.True(t, ok)
	assert.ElementsMatch(t, []string{"automation.create", "automation.update", "automation.delete", "automation.archive", "automation.unarchive"}, registered.AuditActions)
}
