package testhelper_test

import (
	"testing"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSiteActorFixtureUser(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	res, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteContactsList(ctx, siteapi.SiteContactsListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsListOK{}, res)

	// A user with no membership cannot see the Workspace.
	res, err = env.SiteActor(t, fixtures.OutsiderOscarEmail).SiteContactsList(ctx, siteapi.SiteContactsListParams{Slug: fixtures.AcmeSlug})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteContactsListNotFound{}, res)
}

func TestSiteAnonymousIsRejected(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteContactsList(t.Context(), siteapi.SiteContactsListParams{Slug: fixtures.AcmeSlug})
	require.Error(t, err)
}

func TestExternalAnchorToken(t *testing.T) {
	env := testhelper.Setup(t)

	res, err := env.ExternalAnchor(t).AuthTokensList(t.Context())
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ApiTokenListResponse{}, res)
}

func TestExternalScopedTokenEnforcesExactlyItsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	res, err := env.ExternalScoped(t, "contacts:read").AuthTokensList(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensListForbidden{}, res, "contacts:read does not grant tokens:read")

	res, err = env.ExternalScoped(t, "tokens:read").AuthTokensList(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ApiTokenListResponse{}, res)
}

func TestExternalWrongOrMissingCredentialIsRejected(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	res, err := env.ExternalAnonymous(t).AuthTokensList(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensListUnauthorized{}, res)

	res, err = env.ExternalWithToken(t, "omtk_nope_wrong").AuthTokensList(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensListUnauthorized{}, res)
}

func TestCollectKeyActor(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	res, err := env.CollectAcme(t).CollectEventsCreate(ctx, &collectapi.CollectEventsInput{})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateNoContent{}, res)

	res, err = env.CollectAnonymous(t).CollectEventsCreate(ctx, &collectapi.CollectEventsInput{})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateUnauthorized{}, res)

	res, err = env.CollectWithKey(t, "wrong-key").CollectEventsCreate(ctx, &collectapi.CollectEventsInput{})
	require.NoError(t, err)
	assert.IsType(t, &collectapi.CollectEventsCreateUnauthorized{}, res)
}
