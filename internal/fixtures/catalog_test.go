package fixtures_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/membership"
	"github.com/getsphericon/sphericon/ent/user"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The catalog's plaintext secrets must verify against the hashes the fixture
// loader actually stored, through the production verify code.
func TestCatalogPlaintextSecretsVerifyAgainstLoadedHashes(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	for _, tc := range []struct {
		id       int64
		password string
	}{
		{fixtures.OwnerJohnID, fixtures.OwnerJohnPassword},
		{fixtures.OwnerJaneID, fixtures.OwnerJanePassword},
		{fixtures.MemberMaryID, fixtures.MemberMaryPassword},
		{fixtures.OutsiderOscarID, fixtures.OutsiderOscarPassword},
	} {
		u := env.DB.User.GetX(ctx, tc.id)
		require.True(t, credentials.VerifyPassword(u.PasswordHash, tc.password), "user %d", tc.id)
		require.False(t, credentials.VerifyPassword(u.PasswordHash, tc.password+"x"), "user %d", tc.id)
	}

	tok := env.DB.ApiToken.GetX(ctx, fixtures.AnchorTokenID)
	require.True(t, credentials.VerifyTokenSecret(fixtures.AnchorTokenSecret, tok.SecretHash))
	require.False(t, credentials.VerifyTokenSecret(fixtures.AnchorTokenSecret+"x", tok.SecretHash))

	parsed := credentials.ParseToken(credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.AnchorTokenSecret))
	require.NotNil(t, parsed)
	require.Equal(t, fixtures.AnchorTokenPrefix, parsed.Prefix)
	require.Equal(t, 1, env.DB.ApiToken.Query().Where(apitoken.Prefix(parsed.Prefix)).CountX(ctx))
}

func TestAnchorTenancyRows(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()

	role := func(userID, workspaceID int64) membership.Role {
		return env.DB.Membership.Query().
			Where(membership.UserID(userID), membership.WorkspaceID(workspaceID)).
			OnlyX(ctx).Role
	}

	require.Equal(t, membership.RoleOwner, role(fixtures.OwnerJaneID, fixtures.GlobexID))
	require.Equal(t, membership.RoleMember, role(fixtures.MemberMaryID, fixtures.AcmeID))
	require.Zero(t, env.DB.Membership.Query().Where(membership.UserID(fixtures.OutsiderOscarID)).CountX(ctx))
	require.Equal(t, fixtures.OutsiderOscarEmail,
		env.DB.User.Query().Where(user.ID(fixtures.OutsiderOscarID)).OnlyX(ctx).Email)
}
