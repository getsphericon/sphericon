package segments_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/getsphericon/sphericon/internal/usecase/segmentsaving"
)

// A Condition saved again in another key order and spacing is the one the Segment holds (ADR
// 0169): the save writes nothing, so it records no Audit entry and moves no revision.
func TestSavingTheSameConditionSpelledOtherwiseChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	save := testhelper.Module[*segmentsaving.UseCase](env)
	ctx := t.Context()
	acting := env.Bus.Act(env.DB.Scoped(fixtures.AcmeID), events.Actor{Kind: events.ActorUser, ID: "7", Name: "John"})
	created, err := save.Create(ctx, acting, segments.CreateInput{Name: "VIPs", Condition: new(testhelper.SegmentCondition(t,
		`{"kind":"group","combinator":"and","entries":[{"kind":"tag","name":"vip","operator":"has"}]}`))})
	require.NoError(t, err)
	audited := len(env.QueuedEvents(t, events.NameAuditEntry))

	saved, err := save.Update(ctx, acting, created.ID, segments.UpdateInput{Revision: created.Revision, Condition: patch.Set(testhelper.SegmentCondition(t,
		`{ "entries": [ { "operator": "has",  "name": "vip", "kind": "tag" } ], "combinator": "and", "kind": "group" }`))})
	require.NoError(t, err)

	assert.Equal(t, created.Revision, saved.Revision, "the revision stays")
	assert.Len(t, env.QueuedEvents(t, events.NameAuditEntry), audited, "no Audit entry")
}

// A Segment with no tests, made with no Condition or with the empty group, holds the empty group
// and matches every Contact of the Workspace (ADR 0169).
func TestASegmentWithNoTestsMatchesEveryContact(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	save := testhelper.Module[*segmentsaving.UseCase](env)
	ctx := t.Context()
	acme := env.DB.Scoped(fixtures.AcmeID)
	everyone, err := acme.Contact().Query().Count(ctx)
	require.NoError(t, err)
	require.Positive(t, everyone)

	for name, cond := range map[string]*conditiontree.Segment{"none": nil, "empty group": {Group: conditiontree.Empty()}} {
		created, err := save.Create(ctx, acme, segments.CreateInput{Name: name, Condition: cond})
		require.NoError(t, err)

		assert.JSONEq(t, `{"kind":"group","combinator":"and","entries":[]}`, testhelper.Document(t, created.Condition), name)
		count, err := m.Count(ctx, acme, created.ID)
		require.NoError(t, err)
		assert.Equal(t, everyone, count.Count, name)
	}
}
