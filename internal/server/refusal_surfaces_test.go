package server_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	ht "github.com/ogen-go/ogen/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The refusals the server's error handler renders (review rule 15) answer the same status and
// body on /site and /api: the handlers return the module's error and the problem is written once.

// answer is the status and raw body of the last response a client received.
type answer struct {
	status int
	body   string
}

// answerCapture keeps the wire answer, which the typed client decodes away.
type answerCapture struct {
	inner ht.Client
	got   *answer
}

func (c answerCapture) Do(r *http.Request) (*http.Response, error) {
	res, err := c.inner.Do(r)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	*c.got = answer{status: res.StatusCode, body: string(raw)}
	res.Body = io.NopCloser(strings.NewReader(string(raw)))
	return res, nil
}

// capture wraps a transport so that got holds each answer.
func capture(got *answer) func(ht.Client) ht.Client {
	return func(inner ht.Client) ht.Client { return answerCapture{inner: inner, got: got} }
}

func TestARefusedInputAnswers422AlikeOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	var site, ext answer
	siteClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&site))
	extClient := env.ExternalVia(t, env.ScopedBearer(t, string(externalapi.ApiTokenScopeTokensWrite), string(externalapi.ApiTokenScopeContactsRead)), capture(&ext))

	_, err := siteClient.SiteTokensCreate(t.Context(), &siteapi.SiteCreateTokenInput{Name: "   ", Scopes: []siteapi.ApiTokenScope{siteapi.ApiTokenScopeContactsRead}},
		siteapi.SiteTokensCreateParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	_, err = extClient.AuthTokensCreate(t.Context(), &externalapi.CreateApiTokenInput{Name: "   ", Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScopeContactsRead}})
	require.NoError(t, err)

	want := `{"status":422,"title":"Unprocessable Entity","detail":"name must not be empty","errors":{"name":["name must not be empty"]}}`
	assert.Equal(t, http.StatusUnprocessableEntity, site.status)
	assert.JSONEq(t, want, site.body)
	assert.Equal(t, http.StatusUnprocessableEntity, ext.status)
	assert.JSONEq(t, want, ext.body)
}

func TestARemovalOfARowInUseAnswers409AlikeOnBothSurfaces(t *testing.T) {
	env := testhelper.Setup(t)
	var site, ext answer
	siteClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&site))
	extClient := env.ExternalVia(t, env.ScopedBearer(t, string(externalapi.ApiTokenScopeContactsWrite)), capture(&ext))

	_, err := siteClient.SiteTagsDelete(t.Context(), siteapi.SiteTagsDeleteParams{WorkspaceId: fixtures.AcmePublicID, ID: fixtures.TagVipID})
	require.NoError(t, err)
	_, err = extClient.TagsDelete(t.Context(), externalapi.TagsDeleteParams{ID: fixtures.TagVipID})
	require.NoError(t, err)

	assert.Equal(t, http.StatusConflict, site.status)
	assert.Equal(t, http.StatusConflict, ext.status)
	assert.JSONEq(t, site.body, ext.body)
	assert.Contains(t, site.body, `"usedBy":[{"kind":`)
	assert.Contains(t, site.body, `"name":"`+fixtures.RuleVipOnlyName+`"`)
}

// /api has no Workspace in its paths (a token is bound to one), so only /site can address a
// Workspace the caller cannot reach; an unknown one and a foreign one answer alike.
func TestAWorkspaceTheCallerCannotReachAnswers404AlikeOnEveryOperation(t *testing.T) {
	env := testhelper.Setup(t)
	var unknown, foreign answer
	unknownClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&unknown))
	foreignClient := env.SiteActorVia(t, fixtures.OwnerJohnEmail, capture(&foreign))

	_, err := unknownClient.SiteTagsDelete(t.Context(), siteapi.SiteTagsDeleteParams{WorkspaceId: "Unknown0", ID: fixtures.TagVipID})
	require.NoError(t, err)
	_, err = foreignClient.SiteTokensList(t.Context(), siteapi.SiteTokensListParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)

	want := `{"status":404,"title":"Not Found","detail":"workspace not found"}`
	assert.Equal(t, http.StatusNotFound, unknown.status)
	assert.JSONEq(t, want, unknown.body)
	assert.Equal(t, http.StatusNotFound, foreign.status)
	assert.JSONEq(t, want, foreign.body)
}
