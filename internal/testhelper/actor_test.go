package testhelper_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestSiteActorFixtureUser(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	_, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteContactsList(ctx, siteapi.SiteContactsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)

	// A user with no membership cannot see the Workspace.
	_, err = env.SiteActor(t, fixtures.OutsiderOscarEmail).SiteContactsList(ctx, siteapi.SiteContactsListParams{WorkspaceId: fixtures.AcmePublicID})
	testhelper.SiteRefusal(t, err, http.StatusNotFound)
}

func TestSiteAnonymousIsRejected(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.SiteAnonymous(t).SiteContactsList(t.Context(), siteapi.SiteContactsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.Error(t, err)
}

func TestExternalAnchorToken(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := env.ExternalAnchor(t).AuthTokensList(t.Context())
	require.NoError(t, err)
}

func TestExternalScopedTokenEnforcesExactlyItsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	_, err := env.ExternalScoped(t, "contacts:read").AuthTokensList(ctx)
	testhelper.ExternalRefusal(t, err, http.StatusForbidden, "contacts:read does not grant tokens:read")

	_, err = env.ExternalScoped(t, "tokens:read").AuthTokensList(ctx)
	require.NoError(t, err)
}

func TestExternalWrongOrMissingCredentialIsRejected(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	_, err := env.ExternalAnonymous(t).AuthTokensList(ctx)
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)

	_, err = env.ExternalWithToken(t, "omtk_nope_wrong").AuthTokensList(ctx)
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)
}

func TestCollectKeyActor(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	err := env.CollectAcme(t).CollectEventsCreate(ctx, &collectapi.CollectEventsInput{})
	require.NoError(t, err)

	err = env.CollectAnonymous(t).CollectEventsCreate(ctx, &collectapi.CollectEventsInput{})
	testhelper.CollectRefusal(t, err, http.StatusUnauthorized)

	err = env.CollectWithKey(t, "wrong-key").CollectEventsCreate(ctx, &collectapi.CollectEventsInput{})
	testhelper.CollectRefusal(t, err, http.StatusUnauthorized)
}
