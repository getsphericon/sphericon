package external_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/ent/segment"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixture segment 1 ("Active subscribers", rule) is read by id; the list is
// scoped to the token's workspace.
func TestExternalSegmentsRead(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read")

	list, err := c.SegmentsList(ctx, externalapi.SegmentsListParams{})
	require.NoError(t, err)
	ok, isOK := list.(*externalapi.SegmentsListOK)
	require.Truef(t, isOK, "got %T", list)
	total, err := env.DB.Segment.Query().Where(segment.WorkspaceID(fixtures.AcmeID), segment.DiscardedAtIsNil()).Count(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, total, ok.TotalItems)

	got, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: entityIDString(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	seg, isSeg := got.(*externalapi.SegmentResource)
	require.Truef(t, isSeg, "got %T", got)
	assert.Equal(t, "Active subscribers", seg.Name)

	missing, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsGetNotFound{}, missing)
}

// Create and update go through the segments module: a definition that does not fit
// the Contact schema is a 422, and a valid one is stored.
func TestExternalSegmentsWrite(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read", "segments:write")

	bad, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name:       "Bad",
		Definition: testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsCreateUnprocessableEntity{}, bad)

	const def = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`
	created, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name: "Pros", Definition: testhelper.ExternalRuleGroup(t, def),
	})
	require.NoError(t, err)
	seg, isSeg := created.(*externalapi.SegmentResource)
	require.Truef(t, isSeg, "got %T", created)
	assert.Equal(t, "Pros", seg.Name)

	upd, err := c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Name: externalapi.NewOptString("Pros 2")},
		externalapi.SegmentsUpdateParams{ID: seg.ID})
	require.NoError(t, err)
	assert.Equal(t, "Pros 2", upd.(*externalapi.SegmentResource).Name)

	badUpd, err := c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Definition: externalapi.NewOptSegmentRuleGroup(testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))},
		externalapi.SegmentsUpdateParams{ID: seg.ID})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsUpdateUnprocessableEntity{}, badUpd)
}

// Preview matches the segments module's count for the same definition.
func TestExternalSegmentsPreview(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read")

	const def = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`
	want, err := segments.New(env.Bus).Preview(ctx, env.DB.Scoped(1), def)
	require.NoError(t, err)
	res, err := c.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{Definition: externalapi.NewOptSegmentRuleGroup(testhelper.ExternalRuleGroup(t, def))})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.PreviewSegmentResult)
	require.Truef(t, isOK, "got %T", res)
	assert.EqualValues(t, want, ok.Count)

	bad, err := c.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{Definition: externalapi.NewOptSegmentRuleGroup(testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsPreviewUnprocessableEntity{}, bad)
}

func TestExternalSegmentsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	none := env.ExternalScoped(t, "contacts:read")
	list, err := none.SegmentsList(ctx, externalapi.SegmentsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsListUnauthorized{}, list)
	prev, err := none.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsPreviewUnauthorized{}, prev)

	// Read scope cannot write.
	ro := env.ExternalScoped(t, "segments:read")
	cr, err := ro.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name:       "x",
		Definition: testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsCreateUnauthorized{}, cr)
	up, err := ro.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{}, externalapi.SegmentsUpdateParams{ID: entityIDString(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsUpdateUnauthorized{}, up)
	del, err := ro.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: entityIDString(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsArchiveUnauthorized{}, del)

	// Write scope cannot read.
	wo := env.ExternalScoped(t, "segments:write")
	get, err := wo.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: entityIDString(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsGetUnauthorized{}, get)
}

// Another tenant's segment is invisible and immutable through this token.
func TestExternalSegmentsWorkspaceIsolation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	id := externalapi.EntityId(fixtures.SegmentGlobexID)

	c := env.ExternalScoped(t, "segments:read", "segments:write")

	list, err := c.SegmentsList(ctx, externalapi.SegmentsListParams{})
	require.NoError(t, err)
	for _, s := range list.(*externalapi.SegmentsListOK).Items {
		assert.NotEqual(t, fixtures.SegmentGlobexName, s.Name, "another tenant's segment leaked")
	}
	get, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsGetNotFound{}, get)
	upd, err := c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Name: externalapi.NewOptString("pwned")},
		externalapi.SegmentsUpdateParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsUpdateNotFound{}, upd)
	del, err := c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsArchiveNotFound{}, del)
}
