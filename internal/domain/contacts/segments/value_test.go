package segments_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/conditiontree"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// A Condition saved again in another key order and spacing is the one the Segment holds (ADR
// 0169): the save writes nothing, so it records no Audit entry and moves no revision.
func TestSavingTheSameConditionSpelledOtherwiseChangesNothing(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ctx := context.Background()
	acting := env.Bus.Act(env.DB.Scoped(fixtures.AcmeID), events.Actor{Kind: events.ActorUser, ID: "7", Name: "John"})
	created, err := m.Create(ctx, acting, segments.CreateInput{Name: "VIPs", Condition: new(testhelper.Condition(t,
		`{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"tag","operator":"has","value":"vip"}]}`))})
	require.NoError(t, err)
	audited := len(env.QueuedEvents(t, events.NameAuditEntry))

	saved, err := m.Update(ctx, acting, created.ID, segments.UpdateInput{Revision: created.Revision, Condition: patch.Set(testhelper.Condition(t,
		`{ "rules": [ { "value": "vip",  "operator": "has", "field": "tag", "kind": "rule" } ], "combinator": "and", "kind": "group" }`))})
	require.NoError(t, err)

	assert.Equal(t, created.Revision, saved.Revision, "the revision stays")
	assert.Len(t, env.QueuedEvents(t, events.NameAuditEntry), audited, "no Audit entry")
}

// A Segment with no tests, made with no Condition or with the empty group, holds the empty group
// and matches every Contact of the Workspace (ADR 0169).
func TestASegmentWithNoTestsMatchesEveryContact(t *testing.T) {
	env := testhelper.Setup(t)
	m := testhelper.Module[*segments.Module](env)
	ctx := context.Background()
	acme := env.DB.Scoped(fixtures.AcmeID)
	everyone, err := acme.Contact().Query().Count(ctx)
	require.NoError(t, err)
	require.Positive(t, everyone)

	for name, cond := range map[string]*conditiontree.Group{"none": nil, "empty group": new(conditiontree.Empty())} {
		created, err := m.Create(ctx, acme, segments.CreateInput{Name: name, Condition: cond})
		require.NoError(t, err)

		assert.Equal(t, conditiontree.Empty(), created.Condition, name)
		count, err := m.Count(ctx, acme, created.ID)
		require.NoError(t, err)
		assert.Equal(t, everyone, count, name)
	}
}
