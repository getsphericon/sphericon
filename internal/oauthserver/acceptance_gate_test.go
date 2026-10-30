package oauthserver_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The acceptance gate on a User's Connections (ADR 0062): once the fixture terms are in effect,
// MemberMary, who accepted only their stale version, is gated, and a Connection acts as her.

// listContacts is the answer of /api to a contacts list on the bearer.
func listContacts(t *testing.T, env *testhelper.TestEnv, bearer string) externalapi.ContactsListRes {
	t.Helper()
	res, err := env.ExternalWithToken(t, bearer).ContactsList(t.Context(), externalapi.ContactsListParams{})
	require.NoError(t, err)
	return res
}

// mcpRefusal is the status and problem code /mcp refuses the bearer with.
func mcpRefusal(t *testing.T, env *testhelper.TestEnv, bearer string) (status int, code string) {
	t.Helper()
	rec := serve(env, mcpRequest(t, bearer))
	var problem struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem), rec.Body.String())
	return rec.Code, problem.Code
}

// isAcceptancePending reports whether /api refused a contacts list with acceptance_pending.
func isAcceptancePending(res externalapi.ContactsListRes) bool {
	refused, isRefused := res.(*externalapi.ContactsListForbidden)
	return isRefused && refused.Code.Value == externalapi.ProblemCodeAcceptancePending
}

func TestAConnectionOfAGatedUserIsRefusedWithAcceptancePending(t *testing.T) {
	env := testhelper.Setup(t)
	tok := connectAs(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, "contacts:read")
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	api := listContacts(t, env, tok.AccessToken)
	status, code := mcpRefusal(t, env, tok.AccessToken)

	assert.Truef(t, isAcceptancePending(api), "got %#v", api)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, string(externalapi.ProblemCodeAcceptancePending), code)
}

func TestAConnectionWorksAgainOnceItsUserAccepts(t *testing.T) {
	env := testhelper.Setup(t)
	tok := connectAs(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, "contacts:read")
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	refused := listContacts(t, env, tok.AccessToken)
	require.Truef(t, isAcceptancePending(refused), "refused while the terms are pending, got %#v", refused)

	accepted, err := env.SiteActor(t, fixtures.MemberMaryEmail).SiteUserAcceptPending(t.Context(), &siteapi.SiteAcceptPendingInput{
		Items: []siteapi.SiteAcceptance{{Key: fixtures.ServiceTermsKey, Version: fixtures.ServiceTermsCurrentLabel}},
	})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteUserResource{}, accepted)
	api := listContacts(t, env, tok.AccessToken)
	status := mcpStatus(t, env, tok.AccessToken)

	assert.IsType(t, &externalapi.ContactsListOK{}, api)
	assert.NotEqual(t, http.StatusForbidden, status)
}

// The Workspace's API token is no User's (ADR 0061): it keeps working while a member is gated.
func TestAnAPITokenOfTheWorkspaceIsNeverGated(t *testing.T) {
	env := testhelper.Setup(t)
	connection := connectAs(t, env, fixtures.MemberMaryEmail, fixtures.AcmePublicID, "contacts:read")
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	refused := listContacts(t, env, connection.AccessToken)
	require.Truef(t, isAcceptancePending(refused), "the member is gated, got %#v", refused)
	bearer := env.ScopedBearer(t, "contacts:read")

	api := listContacts(t, env, bearer)
	status := mcpStatus(t, env, bearer)

	assert.IsType(t, &externalapi.ContactsListOK{}, api)
	assert.NotEqual(t, http.StatusForbidden, status)
}
