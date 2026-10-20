package external_test

import (
	"context"
	"testing"
	"time"

	"github.com/getsphericon/sphericon/ent/apitoken"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/api/external"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalAuthMeDescribesTheCallingToken(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.ExternalAnchor(t).AuthMeGet(context.Background())
	require.NoError(t, err)
	info, ok := res.(*externalapi.ApiTokenInfo)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, fixtures.AnchorTokenName, info.Name)
	assert.EqualValues(t, fixtures.AnchorTokenID, int64(info.ID))
}

func TestExternalAuthMeRequiresACredential(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.ExternalAnonymous(t).AuthMeGet(context.Background())
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthMeGetUnauthorized{}, res)
}

func TestExternalTokensOfADeletedWorkspaceAreRefused(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).SetDeletedAt(time.Now()).ExecX(ctx)

	res, err := env.ExternalAnchor(t).AuthMeGet(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthMeGetUnauthorized{}, res)

	env.DB.Workspace.UpdateOneID(fixtures.AcmeID).ClearDeletedAt().ExecX(ctx)
	res, err = env.ExternalAnchor(t).AuthMeGet(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ApiTokenInfo{}, res, "restored, the token works again")
}

func TestExternalAuthTokensListIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_ = env.ScopedBearerFor(t, fixtures.GlobexID, "contacts:read")

	res, err := env.ExternalAnchor(t).AuthTokensList(ctx)
	require.NoError(t, err)
	list, ok := res.(*externalapi.ApiTokenListResponse)
	require.Truef(t, ok, "got %T", res)

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

	denied, err := env.ExternalScoped(t, "contacts:read").AuthTokensList(ctx)
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensListForbidden{}, denied)
}

func TestExternalAuthTokensCreate(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalAnchor(t)

	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	res, err := c.AuthTokensCreate(ctx, &externalapi.CreateApiTokenInput{
		Name:      "ci",
		Scopes:    []externalapi.ApiTokenScope{externalapi.ApiTokenScopeContactsRead},
		ExpiresAt: externalapi.NewOptNilTimestamp(externalapi.Timestamp(expires)),
	})
	require.NoError(t, err)
	created, ok := res.(*externalapi.CreateApiTokenResponse)
	require.Truef(t, ok, "got %T", res)
	assert.NotEmpty(t, created.Token)

	row, err := env.DB.ApiToken.Get(ctx, int64(created.TokenInfo.ID))
	require.NoError(t, err)
	assert.EqualValues(t, fixtures.AcmeID, row.WorkspaceID)
	assert.Equal(t, []string{"contacts:read"}, row.Scopes)
	require.NotNil(t, row.ExpiresAt)
	assert.True(t, row.ExpiresAt.Equal(expires))

	// The minted secret authenticates, within its scope only.
	minted := env.ExternalWithToken(t, created.Token)
	list, err := minted.ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsListOK{}, list)
	write, err := minted.ContactsCreate(ctx, &externalapi.CreateContactInput{Email: externalapi.NewOptNilEmailAddress("x@example.com")})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsCreateUnauthorized{}, write)

	denied, err := env.ExternalScoped(t, "tokens:read").AuthTokensCreate(ctx, &externalapi.CreateApiTokenInput{Name: "n"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensCreateForbidden{}, denied)
}

func TestExternalAuthTokensDelete(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalAnchor(t)

	bearer := env.ScopedBearer(t, "contacts:read")
	victim, err := env.DB.ApiToken.Query().Where(apitoken.WorkspaceID(fixtures.AcmeID), apitoken.Name("actor-token")).Only(ctx)
	require.NoError(t, err)

	res, err := c.AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: entityIDString(victim.ID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensDeleteNoContent{}, res)

	revoked, err := env.DB.ApiToken.Get(ctx, victim.ID)
	require.NoError(t, err)
	assert.NotNil(t, revoked.RevokedAt, "delete revokes rather than removes")
	after, err := env.ExternalWithToken(t, bearer).ContactsList(ctx, externalapi.ContactsListParams{})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.ContactsListUnauthorized{}, after, "a revoked token no longer authenticates")

	missing, err := c.AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: 999999})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensDeleteNotFound{}, missing)

	denied, err := env.ExternalScoped(t, "tokens:read").AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: entityIDString(victim.ID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensDeleteForbidden{}, denied)
}

func TestExternalAuthTokensDeleteCannotTouchAnotherWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	_ = env.ScopedBearerFor(t, fixtures.GlobexID, "contacts:read")
	foreign, err := env.DB.ApiToken.Query().Where(apitoken.WorkspaceID(fixtures.GlobexID)).First(ctx)
	require.NoError(t, err)

	res, err := env.ExternalAnchor(t).AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: entityIDString(foreign.ID)})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensDeleteNotFound{}, res)

	still, err := env.DB.ApiToken.Get(ctx, foreign.ID)
	require.NoError(t, err)
	assert.Nil(t, still.RevokedAt)
}

// Bootstrap has no caller token: it is gated by the configured bootstrap secret
// and mints into the oldest workspace. The test env configures none, so the
// handler is built here with one.
func TestExternalAuthTokensBootstrap(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	h := external.NewHandlers(external.Deps{Accounts: accounts.New(env.DB, env.Bus), BootstrapToken: "bootstrap-secret"})
	in := &externalapi.CreateApiTokenInput{Name: "first", Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScopeTokensWrite}}

	wrong, err := h.AuthTokensBootstrap(ctx, in, externalapi.AuthTokensBootstrapParams{XBootstrapToken: "nope"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensBootstrapUnauthorized{}, wrong)

	res, err := h.AuthTokensBootstrap(ctx, in, externalapi.AuthTokensBootstrapParams{XBootstrapToken: "bootstrap-secret"})
	require.NoError(t, err)
	created, ok := res.(*externalapi.CreateApiTokenResponse)
	require.Truef(t, ok, "got %T", res)
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
	h := external.NewHandlers(external.Deps{Accounts: accounts.New(env.DB, env.Bus)})
	blank, err := h.AuthTokensBootstrap(ctx, in, externalapi.AuthTokensBootstrapParams{XBootstrapToken: ""})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensBootstrapUnauthorized{}, blank)
	// Through the real server too.
	res, err := env.ExternalAnonymous(t).AuthTokensBootstrap(ctx, in, externalapi.AuthTokensBootstrapParams{XBootstrapToken: "guess"})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensBootstrapUnauthorized{}, res)
}

func TestExternalAuthTokensCreateCannotGrantAScopeTheCallerLacks(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalScoped(t, "tokens:write", "contacts:read")

	before, err := env.DB.ApiToken.Query().Count(ctx)
	require.NoError(t, err)

	res, err := c.AuthTokensCreate(ctx, &externalapi.CreateApiTokenInput{
		Name:   "escalate",
		Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScopeContactsRead, externalapi.ApiTokenScopeEmailsSend},
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensCreateForbidden{}, res)

	after, err := env.DB.ApiToken.Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a refused mint creates no token")
}

func TestExternalAuthTokensCreateRefusesABlankName(t *testing.T) {
	env := testhelper.Setup(t)
	res, err := env.ExternalAnchor(t).AuthTokensCreate(context.Background(), &externalapi.CreateApiTokenInput{
		Name:   "   ",
		Scopes: []externalapi.ApiTokenScope{externalapi.ApiTokenScopeContactsRead},
	})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensCreateBadRequest{}, res)
}

func TestExternalAuthTokensDeleteTwiceIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	c := env.ExternalAnchor(t)
	env.ScopedBearer(t, "contacts:read")
	victim, err := env.DB.ApiToken.Query().Where(apitoken.WorkspaceID(fixtures.AcmeID), apitoken.Name("actor-token")).Only(ctx)
	require.NoError(t, err)
	id := entityIDString(victim.ID)

	first, err := c.AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensDeleteNoContent{}, first)

	second, err := c.AuthTokensDelete(ctx, externalapi.AuthTokensDeleteParams{ID: id})
	require.NoError(t, err)
	assert.IsType(t, &externalapi.AuthTokensDeleteNotFound{}, second, "an already-revoked token is not found, as on /site")
}
