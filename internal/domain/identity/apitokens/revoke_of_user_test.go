package apitokens_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// RevokeOfUser retires the User's own credentials in the Workspace, and no one else's: another
// member's Connection and the Workspace's API token keep working (ADR 0170).
func TestRevokeOfUserRetiresOnlyThatUsersCredentials(t *testing.T) {
	env := testhelper.Setup(t)
	johns := env.ExternalWithToken(t, env.ConnectionBearer(t, fixtures.OwnerJohnEmail, apitokens.ScopeContactsRead))
	marys := env.ExternalWithToken(t, env.ConnectionBearer(t, fixtures.MemberMaryEmail, apitokens.ScopeContactsRead))
	workspaces := env.ExternalWithToken(t, env.ScopedBearer(t, apitokens.ScopeContactsRead))

	err := testhelper.Module[*apitokens.Module](env).RevokeOfUser(t.Context(), env.DB.Scoped(fixtures.AcmeID), fixtures.OwnerJohnID)

	require.NoError(t, err)
	_, err = johns.ContactsList(t.Context(), externalapi.ContactsListParams{})
	testhelper.ExternalRefusal(t, err, http.StatusUnauthorized)
	_, err = marys.ContactsList(t.Context(), externalapi.ContactsListParams{})
	require.NoError(t, err, "another member's Connection works")
	_, err = workspaces.ContactsList(t.Context(), externalapi.ContactsListParams{})
	assert.NoError(t, err, "the Workspace's API token works")
}
