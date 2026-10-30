package site_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	ht "github.com/ogen-go/ogen/http"

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

func TestSiteSegmentsPreviewEventCondition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.DunderOwnerDwightEmail)
	ctx := context.Background()

	// DunderEventPreview signed up a day ago; no other Dunder contact did.

	def := `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"event:signed_up","operator":"performed","value":"30"}]}`
	out, err := c.SiteSegmentsPreview(ctx, &siteapi.SitePreviewSegmentInput{
		Condition: siteapi.NewOptSiteConditionGroup(testhelper.SiteRuleGroup(t, def)),
	}, siteapi.SiteSegmentsPreviewParams{WorkspaceId: fixtures.DunderPublicID})
	require.NoError(t, err)
	res, ok := out.(*siteapi.SitePreviewSegmentResult)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, int32(1), res.Count)
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
