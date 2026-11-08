package apitokens_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/domain/identity/workspaces"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func TestMintStoresAHashedTokenAndReturnsTheValueOnce(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	expires := time.Now().Add(time.Hour)

	got, err := apitokens.Mint(ctx, s, apitokens.Input{
		Name: "  CI  ", Scopes: []string{"contacts:read", "contacts:read", "events:write"}, ExpiresAt: &expires,
	})
	require.NoError(t, err)

	assert.Equal(t, "CI", got.Token.Name, "name is trimmed")
	assert.Equal(t, []string{"contacts:read", "events:write"}, got.Token.Scopes, "scopes are de-duplicated")
	assert.NotNil(t, got.Token.ExpiresAt)
	assert.Equal(t, int64(fixtures.AcmeID), got.Token.WorkspaceID)

	parsed := credentials.ParseToken(got.Value)
	require.NotNil(t, parsed, "the value is a well-formed token")
	assert.Equal(t, got.Token.Prefix, parsed.Prefix)
	assert.True(t, credentials.VerifySecret(parsed.Secret, got.Token.SecretHash))
	assert.NotContains(t, got.Token.SecretHash, parsed.Secret)
}

func TestMintRefusesABlankNameAndUnknownScopes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	_, err := apitokens.Mint(ctx, s, apitokens.Input{Name: "  ", Scopes: []string{"contacts:read"}})
	blank, ok := errors.AsType[*validation.Error](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, "name", blank.Field)

	_, err = apitokens.Mint(ctx, s, apitokens.Input{Name: "x", Scopes: []string{"contacts:read", "root:all"}})
	unknown, ok := errors.AsType[*validation.Error](err)
	require.True(t, ok, "got %v", err)
	assert.Equal(t, "scopes", unknown.Field)
}

func TestMintWithinNeverGrantsAScopeTheMinterLacks(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)
	minter := []string{"tokens:write", "contacts:read"}

	ok, err := apitokens.MintWithin(ctx, s, minter, apitokens.Input{Name: "child", Scopes: []string{"contacts:read"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"contacts:read"}, ok.Token.Scopes)

	_, err = apitokens.MintWithin(ctx, s, minter, apitokens.Input{Name: "child", Scopes: []string{"contacts:read", "emails:send"}})
	require.ErrorIs(t, err, apitokens.ErrScopeEscalation)

	count, err := s.ApiToken().Query().Count(ctx)
	require.NoError(t, err)
	before := count
	_, err = apitokens.MintWithin(ctx, s, minter, apitokens.Input{Name: "denied", Scopes: []string{"mcp:send"}})
	require.Error(t, err)
	count, err = s.ApiToken().Query().Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, count, "a refused mint creates nothing")
}

func TestRevokeRetiresOnceAndIsWorkspaceScoped(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	s := env.DB.Scoped(fixtures.AcmeID)

	minted, err := apitokens.Mint(ctx, s, apitokens.Input{Name: "short-lived", Scopes: []string{"contacts:read"}})
	require.NoError(t, err)
	id := minted.Token.ID

	require.ErrorIs(t, apitokens.Revoke(ctx, env.DB.Scoped(fixtures.GlobexID), id, time.Now()), apitokens.ErrNotFound,
		"another Workspace cannot revoke it")

	require.NoError(t, apitokens.Revoke(ctx, s, id, time.Now()))
	got, err := s.ApiToken().Get(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, got.RevokedAt, "revoked, not removed")

	require.ErrorIs(t, apitokens.Revoke(ctx, s, id, time.Now()), apitokens.ErrNotFound, "already revoked")
	assert.ErrorIs(t, apitokens.Revoke(ctx, s, 999999, time.Now()), apitokens.ErrNotFound)
}

// bootstrapModule is the module of an Instance whose bootstrap token is configured ("" for none).
func bootstrapModule(env *testhelper.TestEnv, configured string) *apitokens.Module {
	return apitokens.New(env.DB, workspaces.New(env.DB, env.Bus, time.Now), configured, time.Now)
}

// The bootstrap mints the first token, with no caller token to name a Workspace: into the oldest
// one, for the request that presents the configured bootstrap token.
func TestBootstrapMintsTheFirstTokenIntoTheOldestWorkspace(t *testing.T) {
	env := testhelper.Setup(t)
	m := bootstrapModule(env, "boot-secret")
	in := apitokens.Input{Name: "first", Scopes: []string{"tokens:write"}}

	_, err := m.Bootstrap(t.Context(), "another", in)
	require.ErrorIs(t, err, apitokens.ErrBootstrapTokenInvalid)

	minted, err := m.Bootstrap(t.Context(), "boot-secret", in)
	require.NoError(t, err)
	row := env.DB.ApiToken.GetX(t.Context(), minted.Token.ID)
	assert.EqualValues(t, fixtures.AcmeID, row.WorkspaceID, "the oldest Workspace")
	assert.Equal(t, []string{"tokens:write"}, row.Scopes)
	assert.NotEmpty(t, minted.Value)
}

func TestBootstrapAdmitsNoneWithoutAConfiguredToken(t *testing.T) {
	env := testhelper.Setup(t)

	_, err := bootstrapModule(env, "").Bootstrap(t.Context(), "", apitokens.Input{Name: "first"})

	require.ErrorIs(t, err, apitokens.ErrBootstrapTokenInvalid, "an empty token never matches an unset one")
}
