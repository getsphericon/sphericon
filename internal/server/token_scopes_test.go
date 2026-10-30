package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/apitoken"
	"github.com/getsphericon/sphericon/ent/contact"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// The external API checks the scopes each operation declares in the contract before the handler
// runs (ADR 0061): a valid token without one is refused 403, naming what it lacks, and the
// operation does nothing.
func TestATokenWithoutTheScopeOfItsOperationAnswers403(t *testing.T) {
	env := testhelper.Setup(t)
	var got answer
	reader := env.ExternalVia(t, env.ScopedBearer(t, string(externalapi.ApiTokenScopeContactsRead)), capture(&got))

	res, err := reader.ContactsCreate(t.Context(), &externalapi.CreateContactInput{
		Email: externalapi.NewOptNilEmailAddress("scopeless@example.com"),
	})
	require.NoError(t, err)

	assert.IsType(t, &externalapi.ContactsCreateForbidden{}, res)
	assert.Equal(t, http.StatusForbidden, got.status)
	assert.JSONEq(t, `{"status":403,"title":"Forbidden","detail":"the API token lacks the scope contacts:write"}`, got.body)
	assert.Zero(t, env.DB.Contact.Query().Where(contact.Email("scopeless@example.com")).CountX(t.Context()), "the refused call created nothing")
}

// An operation that declares two scopes needs both, and the refusal names each one missing.
func TestATokenWithoutEveryScopeOfItsOperationAnswers403NamingThem(t *testing.T) {
	env := testhelper.Setup(t)
	var got answer
	bare := env.ExternalVia(t, env.ScopedBearer(t, string(externalapi.ApiTokenScopeTemplatesRead)), capture(&got))

	res, err := bare.AutomationsDryRunHttpStep(t.Context(), &externalapi.DryRunHttpStepInput{
		Body: `{}`, ContactId: externalapi.EntityId(fixtures.ContactAliceID),
	})
	require.NoError(t, err)

	assert.IsType(t, &externalapi.AutomationsDryRunHttpStepForbidden{}, res)
	assert.JSONEq(t, `{"status":403,"title":"Forbidden","detail":"the API token lacks the scopes automations:write, contacts:read"}`, got.body)
}

// 401 is only for a credential that is no valid token: missing, malformed, unknown, revoked or
// expired.
func TestAnUnusableTokenAnswers401WhateverItsScopes(t *testing.T) {
	env := testhelper.Setup(t)
	revoked := env.ScopedBearer(t, string(externalapi.ApiTokenScopeContactsRead))
	me, err := env.ExternalWithToken(t, revoked).AuthMeGet(t.Context())
	require.NoError(t, err)
	info, ok := me.(*externalapi.ApiTokenInfo)
	require.True(t, ok, "%T", me)
	gone, err := env.ExternalAnchor(t).AuthTokensDelete(t.Context(), externalapi.AuthTokensDeleteParams{ID: info.ID})
	require.NoError(t, err)
	require.IsType(t, &externalapi.AuthTokensDeleteNoContent{}, gone)

	expired := env.ScopedBearer(t, string(externalapi.ApiTokenScopeContactsRead))
	parsed := credentials.ParseToken(expired)
	require.NotNil(t, parsed)
	n, err := env.DB.ApiToken.Update().Where(apitoken.Prefix(parsed.Prefix)).SetExpiresAt(time.Now().Add(-time.Minute)).Save(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	for name, bearer := range map[string]string{
		"missing":      "",
		"malformed":    "not-a-token",
		"wrong secret": credentials.TokenValue(fixtures.AnchorTokenPrefix, fixtures.ContractTokenSecret),
		"revoked":      revoked,
		"expired":      expired,
	} {
		var got answer
		c := env.ExternalVia(t, bearer, capture(&got))

		res, err := c.ContactsList(t.Context(), externalapi.ContactsListParams{})
		require.NoError(t, err, name)

		assert.IsType(t, &externalapi.ContactsListUnauthorized{}, res, name)
		assert.Equal(t, http.StatusUnauthorized, got.status, name)
	}
}
