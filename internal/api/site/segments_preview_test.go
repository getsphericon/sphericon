package site_test

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	ht "github.com/ogen-go/ogen/http"
	"github.com/samber/lo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteSegmentsPreviewCountsMatchingActiveContacts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	ctx := context.Background()

	// Dunder holds one contact (DunderPreviewTarget) with the distinctive custom field.

	def := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"preview-pro"}]}`
	out, err := c.SiteSegmentsPreview(ctx, &siteapi.SitePreviewSegmentInput{
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, def)),
	}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.DunderPublicID})
	require.NoError(t, err)
	res, ok := out.(*siteapi.SitePreviewSegmentResult)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, int32(1), res.Count)
}

// previewOne previews, in Dunder, a Condition of one leaf.
func previewOne(t *testing.T, c *siteapi.Client, leaf string) siteapi.SiteSegmentsPreviewRes {
	t.Helper()
	def := `{"kind":"group","combinator":"and","rules":[` + leaf + `]}`
	out, err := c.SiteSegmentsPreview(context.Background(), &siteapi.SitePreviewSegmentInput{
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, def)),
	}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.DunderPublicID})
	require.NoError(t, err)
	return out
}

func TestSiteSegmentsPreviewEventCondition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)

	// DunderEventPreview signed up a day ago; no other Dunder contact did.
	once := previewOne(t, c, `{"kind":"event","action":"signed_up","operator":"atLeast","times":1,"withinDays":30}`)
	twice := previewOne(t, c, `{"kind":"event","action":"signed_up","operator":"atLeast","times":2,"withinDays":30}`)

	assert.Equal(t, &siteapi.SitePreviewSegmentResult{Count: 1}, once)
	assert.Equal(t, &siteapi.SitePreviewSegmentResult{Count: 0}, twice, "one signup is not two")
}

// A count or a window outside the contract's range is refused at the edge (400); atLeast 0 is in
// range but the Condition refuses it (422): "not performed" is atMost 0.
func TestSiteSegmentsPreviewRefusesAnEventCountOutOfRange(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)

	negative := previewOne(t, c, `{"kind":"event","action":"signed_up","operator":"atMost","times":-1}`)
	noWindow := previewOne(t, c, `{"kind":"event","action":"signed_up","operator":"atLeast","times":1,"withinDays":0}`)
	atLeastNone := previewOne(t, c, `{"kind":"event","action":"signed_up","operator":"atLeast","times":0}`)

	assert.IsType(t, &siteapi.SiteSegmentsPreviewBadRequest{}, negative)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewBadRequest{}, noWindow)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewUnprocessableEntity{}, atLeastNone)
}

// A Segment stores an event count as it was sent and reads it back.
func TestSiteSegmentsStoreAnEventCount(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	const def = `{"kind":"group","combinator":"and","rules":[{"kind":"event","action":"pricing_viewed","operator":"atLeast","times":2,"withinDays":7}]}`
	sent := testhelper.SiteRuleGroup(t, def)

	out, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: "Pricing twice", Condition: sent},
		siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	created, ok := out.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", out)
	got, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: created.ID})
	require.NoError(t, err)
	stored, ok := got.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", got)

	assert.Equal(t, siteapi.NewOptSiteConditionGroup(sent), stored.Condition)
}

// A number leaf compares a number custom field and a date leaf tests a datetime one: in Dunder,
// DunderPreviewTarget holds 12 seats and renewed two days ago.
func TestSiteSegmentsPreviewComparesNumbersAndDates(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	monthAgo := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339)

	atLeastTen := previewOne(t, c, `{"kind":"number","field":"custom:seats","operator":"gte","value":10}`)
	moreThanTwelve := previewOne(t, c, `{"kind":"number","field":"custom:seats","operator":"gt","value":12}`)
	thisWeek := previewOne(t, c, `{"kind":"dateWithin","field":"custom:renews_at","withinDays":7}`)
	beforeAMonthAgo := previewOne(t, c, `{"kind":"date","field":"custom:renews_at","operator":"before","at":"`+monthAgo+`"}`)

	assert.Equal(t, &siteapi.SitePreviewSegmentResult{Count: 1}, atLeastTen)
	assert.Equal(t, &siteapi.SitePreviewSegmentResult{Count: 0}, moreThanTwelve)
	assert.Equal(t, &siteapi.SitePreviewSegmentResult{Count: 1}, thisWeek)
	assert.Equal(t, &siteapi.SitePreviewSegmentResult{Count: 0}, beforeAMonthAgo)
}

// Each operator fits a field type: a number leaf on a string field or a contact attribute, a date
// leaf on a number field, contains on a number field and eq on a datetime field are refused (422); a
// window of no days is refused at the edge (400).
func TestSiteSegmentsPreviewRefusesANumberOrDateLeafThatDoesNotFit(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)

	numberOnText := previewOne(t, c, `{"kind":"number","field":"custom:plan","operator":"gt","value":1}`)
	numberOnEmail := previewOne(t, c, `{"kind":"number","field":"email","operator":"gt","value":1}`)
	dateOnNumber := previewOne(t, c, `{"kind":"dateWithin","field":"custom:seats","withinDays":7}`)
	containsOnNumber := previewOne(t, c, `{"kind":"rule","field":"custom:seats","operator":"contains","value":"1"}`)
	eqOnDatetime := previewOne(t, c, `{"kind":"rule","field":"custom:renews_at","operator":"eq","value":"x"}`)
	noDays := previewOne(t, c, `{"kind":"dateWithin","field":"custom:renews_at","withinDays":0}`)

	assert.IsType(t, &siteapi.SiteSegmentsPreviewUnprocessableEntity{}, numberOnText)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewUnprocessableEntity{}, numberOnEmail)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewUnprocessableEntity{}, dateOnNumber)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewUnprocessableEntity{}, containsOnNumber)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewUnprocessableEntity{}, eqOnDatetime)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewBadRequest{}, noDays)
}

// A Custom field lists the operators a rule leaf on it takes, and a Condition takes exactly those:
// in Dunder, plan is a string, seats a number and renews_at a datetime field.
func TestSiteCustomFieldsListTheRuleOperatorsAConditionTakes(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	out, err := c.SiteCustomFieldsList(context.Background(), siteapi.SiteCustomFieldsListParams{WorkspaceId: fixtures.DunderPublicID})
	require.NoError(t, err)
	listed, ok := out.(*siteapi.SiteCustomFieldsListOK)
	require.Truef(t, ok, "got %T", out)
	seats, found := lo.Find(listed.Items, func(f siteapi.SiteCustomFieldResource) bool { return f.Key == "seats" })
	require.True(t, found)

	assert.Equal(t, []siteapi.SiteConditionOperator{
		siteapi.SiteConditionOperatorEq, siteapi.SiteConditionOperatorNeq, siteapi.SiteConditionOperatorNotNull,
	}, seats.RuleOperators, "a number field is compared by a number leaf, not by contains")
	for _, f := range listed.Items {
		for _, op := range siteapi.SiteConditionOperator("").AllValues() {
			res := previewOne(t, c, `{"kind":"rule","field":"custom:`+f.Key+`","operator":"`+string(op)+`","value":"x"}`)
			_, refused := res.(*siteapi.SiteSegmentsPreviewUnprocessableEntity)
			assert.Equal(t, !slices.Contains(f.RuleOperators, op), refused, "%s on the %s field %s", op, f.Type, f.Key)
		}
	}
}

// A Segment stores number and date leaves as they were sent and reads them back.
func TestSiteSegmentsStoreNumberAndDateLeaves(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	ctx := context.Background()
	const def = `{"kind":"group","combinator":"and","rules":[` +
		`{"kind":"number","field":"custom:seats","operator":"lte","value":2.5},` +
		`{"kind":"date","field":"custom:renews_at","operator":"after","at":"2026-01-02T03:04:05Z"},` +
		`{"kind":"dateWithin","field":"custom:renews_at","withinDays":30}]}`
	sent := testhelper.SiteRuleGroup(t, def)

	out, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{Name: "Small and renewing", Condition: sent},
		siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.DunderPublicID})
	require.NoError(t, err)
	created, ok := out.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", out)
	got, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.DunderPublicID, ID: created.ID})
	require.NoError(t, err)
	stored, ok := got.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", got)

	assert.Equal(t, siteapi.NewOptSiteConditionGroup(sent), stored.Condition)
}

func TestSiteSegmentsPreviewRejectsInvalidDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	out, err := c.SiteSegmentsPreview(ctx, &siteapi.SitePreviewSegmentInput{
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`)),
	}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewUnprocessableEntity{}, out)
}

func TestSiteSegmentsCreateRejectsInvalidRuleDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	out, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{
		Name:      "Bad rule",
		Condition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsCreateUnprocessableEntity{}, out)
}

func TestSiteSegmentsCreateAndUpdateShareValidation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	bad := testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"nope","operator":"eq","value":"x"}]}`)

	// Create rejects a bad definition.
	created, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{
		Name: "Bad", Condition: bad,
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsCreateUnprocessableEntity{}, created)

	updated, err := c.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{Condition: siteapi.NewOptSiteConditionGroup(bad)},
		siteapi.SiteSegmentsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsUpdateUnprocessableEntity{}, updated)
}

// rawBody replaces the encoded request body with a literal JSON document, so a
// test can send what the typed client cannot express (an explicit null).
type rawBody struct {
	inner ht.Client
	body  string
}

func (r rawBody) Do(req *http.Request) (*http.Response, error) {
	req.Body = io.NopCloser(strings.NewReader(r.body))
	req.ContentLength = int64(len(r.body))
	return r.inner.Do(req)
}

// A Segment is always a Condition: the contract types condition as a non-nullable
// group, so the server rejects an explicit JSON null at the edge (400 from the
// generated request validation) and leaves the stored Condition untouched. An absent
// key keeps the stored Condition.
func TestSiteSegmentsUpdateContractRejectsNullCondition(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	params := siteapi.SiteSegmentsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SegmentActiveID)}
	reader := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before, err := reader.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams(params))
	require.NoError(t, err)
	stored, ok := before.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", before)

	raw := env.SiteActorVia(t, fixtures.OwnerJohnEmail, func(inner ht.Client) ht.Client {
		return rawBody{inner: inner, body: `{"condition": null}`}
	})
	out, err := raw.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{}, params)
	require.NoError(t, err)
	bad, ok := out.(*siteapi.SiteSegmentsUpdateBadRequest)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, siteapi.NewOptInt32(http.StatusBadRequest), bad.Status)

	// Absent condition still updates the name and keeps the Condition.
	out, err = reader.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{Name: siteapi.NewOptString("Renamed")}, params)
	require.NoError(t, err)
	updated, ok := out.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, "Renamed", updated.Name)
	assert.Equal(t, stored.Condition, updated.Condition)
}
