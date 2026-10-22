package site_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Site segments require a valid JWT cookie, like every other site resource.
func TestSiteSegmentsRequireAuth(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteAnonymous(t)

	_, err := c.SiteSegmentsList(context.Background(), siteapi.SiteSegmentsListParams{})
	require.Error(t, err)
}

// The Acme fixture workspace owns two seeded segments. Listing, creating,
// reading, updating and deleting are all scoped to the authenticated user's
// workspace; an unknown workspaceID resolves to 404 instead of leaking data.
func TestSiteSegmentsScopedToWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()

	// A seeded segment of the owned workspace is fetchable by id (selection by key).
	seeded, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.SegmentActiveID)})
	require.NoError(t, err)
	seededRes, ok := seeded.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", seeded)
	assert.Equal(t, "Active subscribers", seededRes.Name)

	// Create scopes the segment to the workspace and returns the resource.
	created, err := c.SiteSegmentsCreate(ctx, &siteapi.SiteCreateSegmentInput{
		Name:      "VIP customers",
		Condition: testhelper.SiteRuleGroup(t, `{"kind":"group","combinator":"and","rules":[{"kind":"rule","field":"custom:plan","operator":"eq","value":"vip"}]}`),
	}, siteapi.SiteSegmentsCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	res, ok := created.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", created)
	assert.Equal(t, "VIP customers", res.Name)

	// The new segment is readable by id.
	got, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	gotRes, ok := got.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", got)
	assert.Equal(t, res.ID, gotRes.ID)

	// Update changes mutable fields.
	updated, err := c.SiteSegmentsUpdate(ctx, &siteapi.SiteUpdateSegmentInput{
		Name: siteapi.NewOptString("VIP renamed"),
	}, siteapi.SiteSegmentsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: res.ID})
	require.NoError(t, err)
	updRes, ok := updated.(*siteapi.SiteSegmentResource)
	require.Truef(t, ok, "got %T", updated)
	assert.Equal(t, "VIP renamed", updRes.Name)

	// An unknown / non-owned workspace workspaceID resolves to 404, not a data leak.
	missing, err := c.SiteSegmentsList(ctx, siteapi.SiteSegmentsListParams{WorkspaceId: "NoSuchWs"})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteSegmentsListNotFound{}, missing)
}

// bodyCapture records the raw response bodies so tests can assert on the wire
// format, which the typed client cannot show (it drops unknown fields).
type bodyCapture struct {
	inner interface {
		Do(*http.Request) (*http.Response, error)
	}
	bodies []string
}

func (b *bodyCapture) Do(r *http.Request) (*http.Response, error) {
	resp, err := b.inner.Do(r)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	b.bodies = append(b.bodies, string(raw))
	resp.Body = io.NopCloser(strings.NewReader(string(raw)))
	return resp, nil
}

// A Segment is always a rule: responses carry no type field.
func TestSiteSegmentResponsesHaveNoType(t *testing.T) {
	env := testhelper.Setup(t)
	var capture *bodyCapture
	c := env.SiteActorVia(t, fixtures.OwnerJohnEmail, func(inner ht.Client) ht.Client {
		capture = &bodyCapture{inner: inner}
		return capture
	})
	ctx := context.Background()

	_, err := c.SiteSegmentsGet(ctx, siteapi.SiteSegmentsGetParams{WorkspaceId: fixtures.AcmePublicID, ID: 1})
	require.NoError(t, err)
	_, err = c.SiteSegmentsList(ctx, siteapi.SiteSegmentsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	require.Len(t, capture.bodies, 2)
	for _, body := range capture.bodies {
		assert.Contains(t, body, `"name"`)
		assert.NotContains(t, body, `"type"`)
	}
}
