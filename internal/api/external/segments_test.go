package external_test

import (
	"context"
	"testing"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/samber/lo"
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
	names := lo.Map(ok.Items, func(s externalapi.SegmentResource, _ int) string { return s.Name })
	assert.Contains(t, names, fixtures.SegmentActiveName)
	assert.NotContains(t, names, fixtures.SegmentGlobexName)

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
		Name:      "Bad",
		Condition: testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsCreateUnprocessableEntity{}, bad)

	const def = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`
	created, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name: "Pros", Condition: testhelper.ExternalRuleGroup(t, def),
	})
	require.NoError(t, err)
	seg, isSeg := created.(*externalapi.SegmentResource)
	require.Truef(t, isSeg, "got %T", created)
	assert.Equal(t, "Pros", seg.Name)

	upd, err := c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Name: externalapi.NewOptString("Pros 2")},
		externalapi.SegmentsUpdateParams{ID: seg.ID})
	require.NoError(t, err)
	assert.Equal(t, "Pros 2", upd.(*externalapi.SegmentResource).Name)

	badUpd, err := c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))},
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
	res, err := c.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, def))})
	require.NoError(t, err)
	ok, isOK := res.(*externalapi.PreviewSegmentResult)
	require.Truef(t, isOK, "got %T", res)
	assert.EqualValues(t, want, ok.Count)

	bad, err := c.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsPreviewUnprocessableEntity{}, bad)
}

// An event count leaf is evaluated, and refused out of range, as on /site: a count or a window
// outside the contract is a 400, atLeast 0 a 422.
func TestExternalSegmentsPreviewCountsEvents(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "segments:read")
	preview := func(leaf string) externalapi.SegmentsPreviewRes {
		t.Helper()
		// Liam did rule_url three times yesterday.
		def := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"email","operator":"eq","value":"` +
			fixtures.ContactLiamEmail + `"},` + leaf + `]}`
		out, err := c.SegmentsPreview(context.Background(), &externalapi.PreviewSegmentInput{
			Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, def)),
		})
		require.NoError(t, err)
		return out
	}

	three := preview(`{"kind":"event","action":"rule_url","operator":"atLeast","times":3,"withinDays":7}`)
	four := preview(`{"kind":"event","action":"rule_url","operator":"atLeast","times":4}`)
	negative := preview(`{"kind":"event","action":"rule_url","operator":"atMost","times":-1}`)
	noWindow := preview(`{"kind":"event","action":"rule_url","operator":"atMost","times":1,"withinDays":0}`)
	atLeastNone := preview(`{"kind":"event","action":"rule_url","operator":"atLeast","times":0}`)

	assert.Equal(t, &externalapi.PreviewSegmentResult{Count: 1}, three)
	assert.Equal(t, &externalapi.PreviewSegmentResult{Count: 0}, four)
	assert.IsType(t, &externalapi.SegmentsPreviewBadRequest{}, negative)
	assert.IsType(t, &externalapi.SegmentsPreviewBadRequest{}, noWindow)
	assert.IsType(t, &externalapi.SegmentsPreviewUnprocessableEntity{}, atLeastNone)
}

// A Segment created with an event count reads back as it was sent.
func TestExternalSegmentsStoreAnEventCount(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read", "segments:write")
	sent := testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"event","action":"pricing_viewed","operator":"atMost","times":0,"withinDays":30}]}`)

	out, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{Name: "Never priced", Condition: sent})
	require.NoError(t, err)
	created, ok := out.(*externalapi.SegmentResource)
	require.Truef(t, ok, "got %T", out)
	got, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: created.ID})
	require.NoError(t, err)
	stored, ok := got.(*externalapi.SegmentResource)
	require.Truef(t, ok, "got %T", got)

	assert.Equal(t, externalapi.NewOptConditionGroup(sent), stored.Condition)
}

func TestExternalSegmentsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	none := env.ExternalScoped(t, "contacts:read")
	list, err := none.SegmentsList(ctx, externalapi.SegmentsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsListForbidden{}, list)
	prev, err := none.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsPreviewForbidden{}, prev)

	// Read scope cannot write.
	ro := env.ExternalScoped(t, "segments:read")
	cr, err := ro.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name:      "x",
		Condition: testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`),
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsCreateForbidden{}, cr)
	up, err := ro.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{}, externalapi.SegmentsUpdateParams{ID: entityIDString(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsUpdateForbidden{}, up)
	del, err := ro.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: entityIDString(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsArchiveForbidden{}, del)

	// Write scope cannot read.
	wo := env.ExternalScoped(t, "segments:write")
	get, err := wo.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: entityIDString(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.SegmentsGetForbidden{}, get)
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
