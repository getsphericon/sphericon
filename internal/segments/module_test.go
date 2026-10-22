package segments_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	badCondition = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"email","operator":"weird","value":"x"}]}`
)

func TestCreateRejectsInvalidDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New(env.Bus)
	ctx := context.Background()

	_, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Bad", Condition: new(badCondition)})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)

	persisted, err := env.DB.Scoped(fixtures.AcmeID).Segment().Query().Where(segment.Name("Bad")).Exist(ctx)
	require.NoError(t, err)
	assert.False(t, persisted, "nothing persisted")
}

func TestCreateAcceptsValidAndEmptyDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New(env.Bus)
	ctx := context.Background()

	def := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"email","operator":"contains","value":"x"}]}`
	s, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Good", Condition: new(def)})
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, s.WorkspaceID)
	assert.Equal(t, def, *s.Condition)

	_, err = m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Empty", Condition: new("")})
	require.NoError(t, err)
	_, err = m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Nil"})
	require.NoError(t, err)
}

func TestUpdateValidatesLikeCreate(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New(env.Bus)
	ctx := context.Background()

	_, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Condition: new(badCondition)})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)

	// An invalid definition is rejected and leaves the row untouched.
	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Condition: new(badCondition)})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)
	s, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.NotEqual(t, badCondition, *s.Condition)

	got, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Name: new("Renamed")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Name)
}

func TestUpdateIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New(env.Bus)

	_, err := m.Update(context.Background(), env.DB.Scoped(fixtures.GlobexID), fixtures.SegmentProPlanID, segments.UpdateInput{Name: new("x")})
	require.ErrorIs(t, err, segments.ErrNotFound)
}

func TestPreviewAndCountAgree(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New(env.Bus)
	ctx := context.Background()

	stored, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	pred, err := condition.ContactPredicate(ctx, env.DB.Scoped(fixtures.AcmeID), *stored.Condition)
	require.NoError(t, err)
	want, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), pred).Count(ctx)
	require.NoError(t, err)
	require.Positive(t, want, "fixture segment should match someone")

	preview, err := m.Preview(ctx, env.DB.Scoped(fixtures.AcmeID), *stored.Condition)
	require.NoError(t, err)
	assert.Equal(t, want, preview)

	count, err := m.Count(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.Equal(t, want, count)
}

func TestPreviewAndCountErrors(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New(env.Bus)
	ctx := context.Background()

	_, err := m.Preview(ctx, env.DB.Scoped(fixtures.AcmeID), badCondition)
	require.ErrorIs(t, err, segments.ErrInvalidCondition)

	_, err = m.Count(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.SegmentProPlanID)
	require.ErrorIs(t, err, segments.ErrNotFound)
}
