package scopedclient_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthcode"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// An authorization code is Workspace-owned: the scope of another Workspace cannot claim it, and a
// code cannot be issued in one Workspace for the Membership of another.
func TestAnOAuthCodeOfOneWorkspaceCannotBeClaimedInAnother(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := t.Context()
	globexScope := env.DB.Scoped(globex)

	claimed, err := globexScope.OAuthCode().Update().
		Where(oauthcode.ID(fixtures.OAuthCodeAcmeID), oauthcode.UsedAtIsNil()).
		SetUsedAt(time.Now()).
		Save(ctx)
	require.NoError(t, err)
	code, err := env.DB.OAuthCode.Get(ctx, fixtures.OAuthCodeAcmeID)
	require.NoError(t, err)
	assert.Zero(t, claimed)
	assert.Nil(t, code.UsedAt, "Acme's code is left unused")

	// test writes: issuing a code across Workspaces is the operation under test.
	_, err = globexScope.OAuthCode().Create().
		SetCodeHash("a-code-for-acme-in-globex").
		SetClientID(code.ClientID).
		SetMembershipID(fixtures.AcmeOwnerMembershipID).
		SetRedirectURI(code.RedirectURI).
		SetCodeChallenge(code.CodeChallenge).
		SetExpiresAt(time.Now().Add(time.Minute)).
		Save(ctx)
	require.ErrorIs(t, err, ent.ErrNotInWorkspace)
}
