package segments_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/contact"
	"github.com/getsphericon/sphericon/ent/segment"
	"github.com/getsphericon/sphericon/internal/domain/contacts/condition"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
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

	_, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Condition: patch.Set(badCondition)})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)

	// An invalid definition is rejected and leaves the row untouched.
	_, err = m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Condition: patch.Set(badCondition)})
	require.ErrorIs(t, err, segments.ErrInvalidCondition)
	s, err := env.DB.Segment.Get(ctx, fixtures.SegmentProPlanID)
	require.NoError(t, err)
	assert.NotEqual(t, badCondition, *s.Condition)

	got, err := m.Update(ctx, env.DB.Scoped(fixtures.AcmeID), fixtures.SegmentProPlanID, segments.UpdateInput{Name: patch.Set("Renamed")})
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Name)
}

func TestUpdateIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New(env.Bus)

	_, err := m.Update(context.Background(), env.DB.Scoped(fixtures.GlobexID), fixtures.SegmentProPlanID, segments.UpdateInput{Name: patch.Set("x")})
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

// A stored Segment "can be texted" counts and lists, live, the Contacts whose phone has an SMS
// Confirmation and no SMS opt-out (fixtures: Tyrell's eligibility matrix, where only "confirmed"
// has both), and its negation the rest of the Workspace.
func TestASegmentOfContactsThatCanBeTextedCountsAndListsThem(t *testing.T) {
	env := testhelper.Setup(t)
	m := segments.New(env.Bus)
	ctx := context.Background()
	ws := env.DB.Scoped(fixtures.TyrellID)
	listed := func(id int64) (int, []int64) {
		t.Helper()
		count, err := m.Count(ctx, ws, id)
		require.NoError(t, err)
		page, err := m.Contacts(ctx, ws, id, pagination.Params{Page: 1, PageSize: 100})
		require.NoError(t, err)
		ids := make([]int64, len(page.Items))
		for i, c := range page.Items {
			ids[i] = c.ID
		}
		return count, ids
	}
	all, err := ws.Contact().Query().Count(ctx)
	require.NoError(t, err)

	count, ids := listed(fixtures.SegmentTyrellTextableID)
	assert.Equal(t, 1, count)
	assert.Equal(t, []int64{fixtures.TyrellMatrixConfirmedID}, ids)
	count, ids = listed(fixtures.SegmentTyrellUntextableID)
	assert.Equal(t, all-1, count)
	assert.Contains(t, ids, int64(fixtures.TyrellMatrixCleanID), "a phone without a Confirmation")
	assert.Contains(t, ids, int64(fixtures.TyrellMatrixUnsubEverythingID), "a confirmed phone that opted out")
	assert.Contains(t, ids, int64(fixtures.TyrellMatrixSuppressedID), "no phone")
	assert.NotContains(t, ids, int64(fixtures.TyrellMatrixConfirmedID))
}
