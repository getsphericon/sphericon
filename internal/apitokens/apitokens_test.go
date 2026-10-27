package apitokens_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
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
	require.ErrorIs(t, err, apitokens.ErrNameEmpty)

	_, err = apitokens.Mint(ctx, s, apitokens.Input{Name: "x", Scopes: []string{"contacts:read", "root:all"}})
	assert.ErrorIs(t, err, apitokens.ErrUnknownScope)
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

	require.ErrorIs(t, apitokens.Revoke(ctx, env.DB.Scoped(fixtures.GlobexID), id), apitokens.ErrNotFound,
		"another Workspace cannot revoke it")

	require.NoError(t, apitokens.Revoke(ctx, s, id))
	got, err := s.ApiToken().Get(ctx, id)
	require.NoError(t, err)
	assert.NotNil(t, got.RevokedAt, "revoked, not removed")

	require.ErrorIs(t, apitokens.Revoke(ctx, s, id), apitokens.ErrNotFound, "already revoked")
	assert.ErrorIs(t, apitokens.Revoke(ctx, s, 999999), apitokens.ErrNotFound)
}
