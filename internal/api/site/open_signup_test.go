package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// selfHosted is an Instance in the selfhosted Mode, which already has its Owners (the fixtures).
func selfHosted(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	return testhelper.Setup(t, testhelper.WithMode(config.ModeSelfHosted))
}

// ADR 0063: sign-up is always open, so a second person signs up on a self-hosted Instance that
// already has an Owner and works in a first Workspace of their own.
func TestASecondPersonSignsUpOnASelfHostedInstanceAndGetsTheirOwnWorkspace(t *testing.T) {
	env := selfHosted(t)

	res := signUpWithPassword(t, env, "Grace Hopper", "grace@example.com", formPassword)

	require.IsType(t, &siteapi.SiteAuthSignUpAccepted{}, res)
	rec := postLogin(t, env, "grace@example.com", formPassword)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	spaces, err := env.SiteWithToken(t, sessionCookie(rec.Result()).Value).SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, accounts.FirstWorkspaceName, spaces[0].Name)
}

func TestAnOwnerMadeByCreateOwnerSignsIn(t *testing.T) {
	env := selfHosted(t)

	password, err := accounts.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords).CreateOwner(t.Context(), "ada@example.com", "Ada Lovelace")
	require.NoError(t, err)

	rec := postLogin(t, env, "ada@example.com", password)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotNil(t, sessionCookie(rec.Result()))
	var body siteapi.SiteLoginResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, siteapi.SiteLoginOutcomeSession, body.Outcome)
}
