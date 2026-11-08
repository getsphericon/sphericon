package external_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/apitoken"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestExternalAuthMeDescribesTheCallingToken(t *testing.T) {
	env := testhelper.Setup(t)
	info, err := env.ExternalAnchor(t).AuthMeGet(context.Background())
	require.NoError(t, err)
	assert.Equal(t, fixtures.AnchorTokenName, info.Name)
	assert.EqualValues(t, fixtures.AnchorTokenID, int64(info.ID))
}

func TestExternalAuthMeRequiresACredential(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.ExternalAnonymous(t).AuthMeGet(context.Background())
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)
}

func TestExternalTokensOfADeletedWorkspaceAreRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).SetDiscardedAt(time.Now()).ExecX(ctx)

	_, err := env.ExternalAnchor(t).AuthMeGet(ctx)
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)

	env.FeedDB.Workspace.UpdateOneID(fixtures.AcmeID).ClearDiscardedAt().ExecX(ctx)
	_, err = env.ExternalAnchor(t).AuthMeGet(ctx)
	require.NoError(t, err, "restored, the token works again")
}

func TestExternalAuthTokensListIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	list, err := env.ExternalAnchor(t).AuthTokensList(ctx)
	require.NoError(t, err)

	acme, err := env.DB.ApiToken.Query().Where(apitoken.WorkspaceID(fixtures.AcmeID)).Count(ctx)
	require.NoError(t, err)
	assert.Len(t, list.Items, acme, "exactly the caller's workspace tokens")
	globex, err := env.DB.ApiToken.Query().Where(apitoken.WorkspaceID(fixtures.GlobexID)).All(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, globex)
	for _, it := range list.Items {
		for _, g := range globex {
			assert.NotEqual(t, entityIDString(g.ID), it.ID, "another tenant's token leaked")
		}
	}

	_, err = env.ExternalScoped(t, "contacts:read").AuthTokensList(ctx)
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

func TestExternalAuthTokensListIsInCreationOrder(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	list, err := env.ExternalAnchor(t).AuthTokensList(ctx)
	require.NoError(t, err)
	got := lo.Map(list.Items, func(it externalapi.ApiTokenInfo, _ int) int64 { return int64(it.ID) })

	want, err := env.DB.ApiToken.Query().
		Where(apitoken.WorkspaceID(fixtures.AcmeID)).
		Order(ent.Asc(apitoken.FieldID)).
		IDs(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestExternalAuthTokensCreate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalAnchor(t)

	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	created, err := c.AuthTokensCreate(ctx, &externalapi.CreateApiTokenInput{
		Name:      "ci",
		Scopes:    []externalapi.ApiTokenScope{externalapi.ApiTokenScopeContactsRead},
		ExpiresAt: externalapi.NewOptNilTimestamp(externalapi.Timestamp(expires)),
	})
	require.NoError(t, err)
	assert.NotEmpty(t, created.Token)

	row, err := env.DB.ApiToken.Get(ctx, int64(created.TokenInfo.ID))
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, row.WorkspaceID)
	assert.Equal(t, []string{"contacts:read"}, row.Scopes)
	require.NotNil(t, row.ExpiresAt)
	assert.True(t, row.ExpiresAt.Equal(expires))

	// The minted secret authenticates, within its scope only.
	minted := env.ExternalWithToken(t, created.Token)
	_, err = minted.ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	_, err = minted.ContactsCreate(ctx, &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress("x@example.com")})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	_, err = env.ExternalScoped(t, "tokens:read").AuthTokensCreate(ctx, &externalapi.CreateApiTokenInput{Name: "n"})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

func TestExternalAuthTokensDelete(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalAnchor(t)
	victim := entityIDString(fixtures.ContractTokenID)

	err := c.AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: victim})
	require.NoError(t, err)

	revoked, err := env.DB.ApiToken.Get(ctx, fixtures.ContractTokenID)
	require.NoError(t, err)
	assert.NotNil(t, revoked.RevokedAt, "delete revokes rather than removes")
	bearer := credentials.TokenValue(fixtures.ContractTokenPrefix, fixtures.ContractTokenSecret)
	_, err = env.ExternalWithToken(t, bearer).ContactsList(ctx, externalapi.ContactsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized, "a revoked token no longer authenticates")

	err = c.AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: 999999})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	err = env.ExternalScoped(t, "tokens:read").AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: victim})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)
}

func TestExternalAuthTokensDeleteCannotTouchAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()

	err := env.ExternalAnchor(t).AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: entityIDString(fixtures.TokenGlobexID)})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound)

	still, err := env.DB.ApiToken.Get(ctx, fixtures.TokenGlobexID)
	require.NoError(t, err)
	assert.Nil(t, still.RevokedAt)
}

// Bootstrap has no caller token: it is gated by the configured bootstrap secret
// and mints into the oldest workspace. The test env configures none, so the
// handler is built here with one.
func TestExternalAuthTokensBootstrap(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	h := external.NewHandlers(external.Deps{Clock: time.Now, Workspaces: workspaces.New(env.DB, env.Bus, time.Now), BootstrapToken: "bootstrap-secret"})
	in := &externalapi.CreateApiTokenInput{Name: "first", Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScopeTokensWrite}}

	_, err := h.AuthTokensBootstrap(ctx, in, externalapi.AuthTokensBootstrapParams{XBootstrapToken: "nope"})
	require.ErrorIs(t, err, apitokens.ErrBootstrapTokenInvalid)

	created, err := h.AuthTokensBootstrap(ctx, in, externalapi.AuthTokensBootstrapParams{XBootstrapToken: "bootstrap-secret"})
	require.NoError(t, err)
	row, err := env.DB.ApiToken.Get(ctx, int64(created.TokenInfo.ID))
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, row.WorkspaceID, "the oldest workspace")
	assert.Equal(t, []string{"tokens:write"}, row.Scopes)
}

func TestExternalAuthTokensBootstrapDisabledWithoutConfiguredSecret(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	in := &externalapi.CreateApiTokenInput{Name: "first"}

	// Unconfigured: an empty header must not match the empty secret.
	h := external.NewHandlers(external.Deps{Clock: time.Now, Workspaces: workspaces.New(env.DB, env.Bus, time.Now)})
	_, err := h.AuthTokensBootstrap(ctx, in, externalapi.AuthTokensBootstrapParams{XBootstrapToken: ""})
	require.ErrorIs(t, err, apitokens.ErrBootstrapTokenInvalid)
	// Through the real server too.
	_, err = env.ExternalAnonymous(t).AuthTokensBootstrap(ctx, in, externalapi.AuthTokensBootstrapParams{XBootstrapToken: "guess"})
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)
}

func TestExternalAuthTokensCreateCannotGrantAScopeTheCallerLacks(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "tokens:write", "contacts:read")

	before, err := env.DB.ApiToken.Query().Count(ctx)
	require.NoError(t, err)

	_, err = c.AuthTokensCreate(ctx, &externalapi.CreateApiTokenInput{
		Name:   "escalate",
		Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScopeContactsRead, externalapi.ApiTokenScopeEmailsSend},
	})
	testhelper.ExternalRefusal(t, err, http.StatusForbidden)

	after, err := env.DB.ApiToken.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a refused mint creates no token")
}

func TestExternalAuthTokensCreateRefusesABlankNameWith422(t *testing.T) {
	env := testhelper.Setup(t)
	_, err := env.ExternalAnchor(t).AuthTokensCreate(context.Background(), &externalapi.CreateApiTokenInput{
		Name:   "   ",
		Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScopeContactsRead},
	})
	testhelper.ExternalRefusal(t, err, http.StatusUnprocessableEntity)
}

func TestExternalAuthTokensDeleteTwiceIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalAnchor(t)
	id := entityIDString(fixtures.ContractTokenID)

	err := c.AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: id})
	require.NoError(t, err)

	err = c.AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: id})
	testhelper.ExternalRefusal(t, err, http.StatusNotFound, "an already-revoked token is not found, as on /site")
}
