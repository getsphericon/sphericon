package site_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	ht "github.com/ogen-go/ogen/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteSegmentsPreviewCountsMatchingActiveContacts(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// An active contact with a distinctive custom field in workspace acme (id 1).
	_, err := env.DB.Contact.Create().
		SetWorkspaceID(fixtures.AcmeID).
		SetEmail("preview-target@test.dev").
		SetCustomFields(map[string]any{"plan": "preview-pro"}).
		Save(ctx)
	require.NoError(t, err)

	def := `{"combinator":"and","rules":[{"field":"custom:plan","operator":"=","value":"preview-pro"}]}`
	out, err := c.SiteSegmentsPreview(ctx, &siteapi.SitePreviewSegmentInput{
		Definition: siteapi.NewOptNilString(def),
	}, siteapi.SiteSegmentsPreviewParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	res, ok := out.(*siteapi.SitePreviewSegmentResult)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, int32(1), res.Count)
}

func TestSiteSegmentsPreviewEventCondition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	ct, err := env.DB.Contact.Create().SetWorkspaceID(fixtures.AcmeID).SetEmail("ev-prev@test.dev").Save(ctx)
	require.NoError(t, err)
	// Events attach by the stable contact_id (ADR 0002), not the email string.
	_, err = env.DB.Event.Create().
		SetWorkspaceID(fixtures.AcmeID).SetContactID(ct.ID).
		SetAction("signed_up").SetOccurredAt(time.Now()).Save(ctx)
	require.NoError(t, err)

	def := `{"combinator":"and","rules":[{"field":"event:signed_up","operator":"performed","value":"30"}]}`
	out, err := c.SiteSegmentsPreview(ctx, &siteapi.SitePreviewSegmentInput{
		Definition: siteapi.NewOptNilString(def),
	}, siteapi.SiteSegmentsPreviewParams{Slug: fixtures.AcmeSlug})
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
		Definition: siteapi.NewOptNilString(`{"rules":[{"field":"nope","operator":"=","value":"x"}]}`),
	}, siteapi.SiteSegmentsPreviewParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsPreviewUnprocessableEntity{}, out)
}

func TestSiteSegmentsCreateRejectsInvalidRuleDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	out, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{
		Name:       "Bad rule",
		Definition: `{"rules":[{"field":"email","operator":"weird","value":"x"}]}`,
	}, siteapi.SiteSegmentsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsCreateUnprocessableEntity{}, out)
}

func TestSiteSegmentsCreateAndUpdateShareValidation(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	bad := `{"rules":[{"field":"email","operator":"weird","value":"x"}]}`

	// Create rejects a bad definition.
	created, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{
		Name: "Bad", Definition: bad,
	}, siteapi.SiteSegmentsCreateParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsCreateUnprocessableEntity{}, created)

	updated, err := c.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{Definition: siteapi.NewOptString(bad)},
		siteapi.SiteSegmentsUpdateParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.SegmentActiveID)})
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

// A Segment is always a rule: the contract types definition as a non-nullable
// string, so the server rejects an explicit JSON null at the edge (400 from the
// generated request validation) and leaves the stored rule untouched. An absent
// key keeps the stored rule.
func TestSiteSegmentsUpdateContractRejectsNullDefinition(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	params := siteapi.SiteSegmentsUpdateParams{Slug: fixtures.AcmeSlug, ID: idStr(fixtures.SegmentActiveID)}
	reader := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before, err := reader.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams(params))
	require.NoError(t, err)
	stored, ok := before.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", before)

	raw := env.SiteActorVia(t, fixtures.OwnerJohnEmail, func(inner ht.Client) ht.Client {
		return rawBody{inner: inner, body: `{"definition": null}`}
	})
	out, err := raw.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{}, params)
	require.NoError(t, err)
	bad, ok := out.(*siteapi.SiteSegmentsUpdateBadRequest)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, siteapi.NewOptInt32(http.StatusBadRequest), bad.Status)

	// Absent definition still updates the name and keeps the rule.
	out, err = reader.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{Name: siteapi.NewOptString("Renamed")}, params)
	require.NoError(t, err)
	updated, ok := out.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", out)
	assert.Equal(t, "Renamed", updated.Name)
	assert.Equal(t, stored.Definition, updated.Definition)
}
