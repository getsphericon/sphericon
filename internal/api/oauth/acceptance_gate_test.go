package oauth_test

import (
	"encoding/json"
	"errors"
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
// Vehement's member Mel, who has accepted none of them, is gated, and her Connection acts as her.

// listContacts is the error /api answers a contacts list on the bearer with: nil when it serves it.
func listContacts(t *testing.T, env *testhelper.TestEnv, bearer string) error {
	t.Helper()
	_, err := env.ExternalWithToken(t, bearer).ContactsList(t.Context(), externalapi.ContactsListParams{})
	return err
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
func isAcceptancePending(err error) bool {
	refused, isRefused := errors.AsType[*externalapi.ProblemDetailsStatusCode](err)
	return isRefused && refused.StatusCode == http.StatusForbidden &&
		refused.Response.Code.Value == externalapi.ProblemCodeAcceptancePending
}

func TestAConnectionOfAGatedUserIsRefusedWithAcceptancePending(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)

	api := listContacts(t, env, melsConnection)
	status, code := mcpRefusal(t, env, melsConnection)

	assert.Truef(t, isAcceptancePending(api), "got %#v", api)
	assert.Equal(t, http.StatusForbidden, status)
	assert.Equal(t, string(externalapi.ProblemCodeAcceptancePending), code)
}

func TestAConnectionWorksAgainOnceItsUserAccepts(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	refused := listContacts(t, env, melsConnection)
	require.Truef(t, isAcceptancePending(refused), "refused while the terms are pending, got %#v", refused)

	_, err := env.SiteActor(t, fixtures.VehementMemberMelEmail).SiteUserAcceptPending(t.Context(), &siteapi.SiteAcceptPendingInput{
		Items: []siteapi.SiteAcceptance{{Key: fixtures.ServiceTermsKey, Version: fixtures.ServiceTermsCurrentLabel}},
	})
	require.NoError(t, err)

	api := listContacts(t, env, melsConnection)
	status := mcpStatus(t, env, melsConnection)

	require.NoError(t, api)
	assert.NotEqual(t, http.StatusForbidden, status)
}

// The Workspace's API token is no User's (ADR 0061): it keeps working while a member is gated.
func TestAnAPITokenOfTheWorkspaceIsNeverGated(t *testing.T) {
	env := testhelper.Setup(t)
	env.PutSignupItemsInEffect(t.Context(), fixtures.ServiceTermsID)
	refused := listContacts(t, env, melsConnection)
	require.Truef(t, isAcceptancePending(refused), "the member is gated, got %#v", refused)
	bearer := env.ScopedBearerFor(t, fixtures.VehementID, "contacts:read")

	api := listContacts(t, env, bearer)
	status := mcpStatus(t, env, bearer)

	require.NoError(t, api)
	assert.NotEqual(t, http.StatusForbidden, status)
}
