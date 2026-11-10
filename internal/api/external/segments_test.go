package external_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/contacts/segments"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Fixture segment 1 ("Active subscribers", rule) is read by id; the list is
// scoped to the token's workspace.
func TestExternalSegmentsRead(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read")

	list, err := c.SegmentsList(ctx, externalapi.SegmentsListParams{})
	require.NoError(t, err)
	names := lo.Map(list.Items, func(s externalapi.SegmentResource, _ int) string { return s.Name })
	assert.Contains(t, names, fixtures.SegmentActiveName)
	assert.NotContains(t, names, fixtures.SegmentGlobexName)

	seg, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: entityIDString(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	assert.Equal(t, "Active subscribers", seg.Name)

	_, err = c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

// Create and update go through the segments module: a definition that does not fit
// the Contact schema is a 422, and a valid one is stored.
func TestExternalSegmentsWrite(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read", "segments:write")

	_, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name:      "Bad",
		Condition: testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`),
	})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)

	const def = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`
	seg, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name: "Pros", Condition: testhelper.ExternalRuleGroup(t, def),
	})
	require.NoError(t, err)
	assert.Equal(t, "Pros", seg.Name)

	upd, err := c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Name: externalapi.NewOptName("Pros 2")},
		externalapi.SegmentsUpdateParams{ID: seg.ID})
	require.NoError(t, err)
	assert.Equal(t, "Pros 2", upd.Name)

	_, err = c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))},
		externalapi.SegmentsUpdateParams{ID: seg.ID})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}

// Preview matches the segments module's count for the same definition.
func TestExternalSegmentsPreview(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read")

	const def = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"pro"}]}`
	want, err := segments.New(env.Bus, time.Now).Preview(ctx, env.DB.Scoped(1), segments.PreviewInput{Condition: new(def)})
	require.NoError(t, err)
	res, err := c.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, def))})
	require.NoError(t, err)
	assert.EqualValues(t, want.Count, res.Count)

	_, err = c.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`))})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}

// An event count leaf is evaluated, and refused out of range, as on /site: a count or a window
// outside the contract is a 400, atLeast 0 a 422.
func TestExternalSegmentsPreviewCountsEvents(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "segments:read")
	preview := func(leaf string) (*externalapi.PreviewSegmentResult, error) {
		t.Helper()
		// Liam did rule_url three times yesterday.
		def := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"email","operator":"eq","value":"` +
			fixtures.ContactLiamEmail + `"},` + leaf + `]}`
		return c.SegmentsPreview(context.Background(), &externalapi.PreviewSegmentInput{
			Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, def)),
		})
	}

	three, err := preview(`{"kind":"event","action":"rule_url","operator":"atLeast","times":3,"withinDays":7}`)
	require.NoError(t, err)
	four, err := preview(`{"kind":"event","action":"rule_url","operator":"atLeast","times":4}`)
	require.NoError(t, err)
	_, negative := preview(`{"kind":"event","action":"rule_url","operator":"atMost","times":-1}`)
	_, noWindow := preview(`{"kind":"event","action":"rule_url","operator":"atMost","times":1,"withinDays":0}`)
	_, atLeastNone := preview(`{"kind":"event","action":"rule_url","operator":"atLeast","times":0}`)

	assert.Equal(t, &externalapi.PreviewSegmentResult{Count: 1}, three)
	assert.Equal(t, &externalapi.PreviewSegmentResult{Count: 0}, four)
	testhelper.ExternalRefusal(t, negative, http.StatusBadRequest)
	testhelper.ExternalRefusal(t, noWindow, http.StatusBadRequest)
	testhelper.ExternalRefusal(t, atLeastNone, http.StatusUnprocessableEntity)
}

// A Segment created with an event count reads back as it was sent.
func TestExternalSegmentsStoreAnEventCount(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read", "segments:write")
	sent := testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"event","action":"pricing_viewed","operator":"atMost","times":0,"withinDays":30}]}`)

	created, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{Name: "Never priced", Condition: sent})
	require.NoError(t, err)
	stored, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: created.ID})
	require.NoError(t, err)

	assert.Equal(t, externalapi.NewOptConditionGroup(sent), stored.Condition)
}

// A number leaf takes a number custom field, a date leaf a datetime one, as on /site: Acme's
// courses_completed is a number and plan a string.
func TestExternalSegmentsPreviewFitsNumberAndDateLeavesToTheFieldType(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.ExternalScoped(t, "segments:read")
	preview := func(leaf string) error {
		t.Helper()
		_, err := c.SegmentsPreview(context.Background(), &externalapi.PreviewSegmentInput{
			Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[`+leaf+`]}`)),
		})
		return err
	}

	onNumber := preview(`{"kind":"number","field":"custom:courses_completed","operator":"gte","value":3}`)
	numberOnText := preview(`{"kind":"number","field":"custom:plan","operator":"gte","value":3}`)
	dateOnNumber := preview(`{"kind":"dateWithin","field":"custom:courses_completed","withinDays":7}`)

	require.NoError(t, onNumber)
	testhelper.ExternalRefusal(t, numberOnText, http.StatusUnprocessableEntity)
	testhelper.ExternalRefusal(t, dateOnNumber, http.StatusUnprocessableEntity)
}

// A Segment created with a number leaf reads back as it was sent.
func TestExternalSegmentsStoreANumberLeaf(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read", "segments:write")
	sent := testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"number","field":"custom:courses_completed","operator":"lt","value":1.5}]}`)

	created, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{Name: "Barely started", Condition: sent})
	require.NoError(t, err)
	stored, err := c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: created.ID})
	require.NoError(t, err)

	assert.Equal(t, externalapi.NewOptConditionGroup(sent), stored.Condition)
}

func TestExternalSegmentsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	none := env.ExternalScoped(t, "contacts:read")
	_, err := none.SegmentsList(ctx, externalapi.SegmentsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = none.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	// Read scope cannot write.
	ro := env.ExternalScoped(t, "segments:read")
	_, err = ro.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{
		Name:      "x",
		Condition: testhelper.ExternalRuleGroup(t, `{"kind":"group","combinator":"and","rules":[]}`),
	})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = ro.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{}, externalapi.SegmentsUpdateParams{ID: entityIDString(fixtures.SegmentActiveID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
	_, err = ro.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: entityIDString(fixtures.SegmentActiveID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	// Write scope cannot read.
	wo := env.ExternalScoped(t, "segments:write")
	_, err = wo.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: entityIDString(fixtures.SegmentActiveID)})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

// Another tenant's segment is invisible and immutable through this token.
func TestExternalSegmentsWorkspaceIsolation(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	id := externalapi.EntityId(fixtures.SegmentGlobexID)

	c := env.ExternalScoped(t, "segments:read", "segments:write")

	list, err := c.SegmentsList(ctx, externalapi.SegmentsListParams{})
	require.NoError(t, err)
	for _, s := range list.Items {
		assert.NotEqual(t, fixtures.SegmentGlobexName, s.Name, "another tenant's segment leaked")
	}
	_, err = c.SegmentsGet(ctx, externalapi.SegmentsGetParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	_, err = c.SegmentsUpdate(ctx, &externalapi.UpdateSegmentInput{Name: externalapi.NewOptName("pwned")},
		externalapi.SegmentsUpdateParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
	_, err = c.SegmentsArchive(ctx, externalapi.SegmentsArchiveParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)
}

// The "can be texted" condition is accepted on /api: a Segment stores it, and a preview counts the
// Texter only once an API client has recorded the consent of the Texter's phone.
func TestExternalSegmentsTakeTheCanBeTextedCondition(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "segments:read", "segments:write", "contacts:write")
	const def = `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"email","operator":"eq","value":"` +
		fixtures.ContactTexterEmail + `"},{"kind":"rule","field":"sms","operator":"has"}]}`
	preview := func() *externalapi.PreviewSegmentResult {
		t.Helper()
		out, err := c.SegmentsPreview(ctx, &externalapi.PreviewSegmentInput{Condition: externalapi.NewOptConditionGroup(testhelper.ExternalRuleGroup(t, def))})
		require.NoError(t, err)
		return out
	}

	_, err := c.SegmentsCreate(ctx, &externalapi.CreateSegmentInput{Name: "Texter", Condition: testhelper.ExternalRuleGroup(t, def)})
	require.NoError(t, err)
	before := preview()
	_, err = c.ConfirmationsCreate(ctx, smsConsent(fixtures.ContactTexterPhone))
	require.NoError(t, err)
	after := preview()

	assert.Equal(t, &externalapi.PreviewSegmentResult{Count: 0}, before)
	assert.Equal(t, &externalapi.PreviewSegmentResult{Count: 1}, after)
}
