package segments_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	badDefinition = `{"rules":[{"field":"email","operator":"weird","value":"x"}]}`
)

func ptr[T any](v T) *T { return &v }

func TestCreateRejectsInvalidDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New()
	ctx := context.Background()

	{
		before, err := env.DB.Segment.Query().Count(ctx)
		require.NoError(t, err)

		_, err = m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Bad", Definition: ptr(badDefinition)})
		require.ErrorIs(t, err, segments.ErrInvalidDefinition)

		after, err := env.DB.Segment.Query().Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, before, after, "nothing persisted")
	}
}

func TestCreateAcceptsValidAndEmptyDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New()
	ctx := context.Background()

	def := `{"combinator":"and","rules":[{"field":"email","operator":"contains","value":"x"}]}`
	s, err := m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Good", Definition: ptr(def)})
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, s.WorkspaceID)
	assert.Equal(t, def, *s.Definition)

	_, err = m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Empty", Definition: ptr("")})
	require.NoError(t, err)
	_, err = m.Create(ctx, env.DB.Scoped(fixtures.AcmeID), segments.CreateInput{Name: "Nil"})
	require.NoError(t, err)
}

func TestUpdateValidatesLikeCreate(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New()
	ctx := context.Background()

	_, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Definition: ptr(badDefinition)})
	require.ErrorIs(t, err, segments.ErrInvalidDefinition)

	// An invalid definition is rejected and leaves the row untouched.
	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Definition: ptr(badDefinition)})
	require.ErrorIs(t, err, segments.ErrInvalidDefinition)
	s, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.NotEqual(t, badDefinition, *s.Definition)

	got, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Name: ptr("Renamed")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Name)
}

func TestUpdateIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New()

	_, err := m.Update(context.Background(), env.DB.Scoped(fixtures.GlobexID), fixtures.SegmentProPlanID, segments.UpdateInput{Name: ptr("x")})
	require.ErrorIs(t, err, segments.ErrNotFound)
}

func TestPreviewAndCountAgree(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New()
	ctx := context.Background()

	stored, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	pred, err := segments.ContactPredicate(ctx, env.DB.Scoped(fixtures.AcmeID), *stored.Definition)
	require.NoError(t, err)
	want, err := env.DB.Contact.Query().Where(contact.WorkspaceID(fixtures.AcmeID), pred).Count(ctx)
	require.NoError(t, err)
	require.Positive(t, want, "fixture segment should match someone")

	preview, err := m.Preview(ctx, env.DB.Scoped(fixtures.AcmeID), *stored.Definition)
	require.NoError(t, err)
	assert.Equal(t, want, preview)

	count, err := m.Count(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.Equal(t, want, count)
}

func TestPreviewAndCountErrors(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New()
	ctx := context.Background()

	_, err := m.Preview(ctx, env.DB.Scoped(fixtures.AcmeID), badDefinition)
	require.ErrorIs(t, err, segments.ErrInvalidDefinition)

	_, err = m.Count(ctx, env.DB.Scoped(fixtures.GlobexID), fixtures.SegmentProPlanID)
	require.ErrorIs(t, err, segments.ErrNotFound)
}
