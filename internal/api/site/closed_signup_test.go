package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/config"
	entuser "github.com/getsphericon/sphericon/ent/user"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func closedInstance(t *testing.T) *testhelper.TestEnv {
	t.Helper()
	return testhelper.Setup(t, testhelper.WithConfig(func(c *config.Config) { c.SignupOpen = false }))
}

func TestAClosedInstanceRefusesSignupAndCreatesNoUser(t *testing.T) {
	env := closedInstance(t)

	res, err := env.SiteAnonymous(t).SiteAuthRegister(t.Context(),
		&siteapi.SiteRegisterInput{Name: "Grace Hopper", Email: "grace@example.com"})
	require.NoError(t, err)
	refused, ok := res.(*siteapi.SiteAuthRegisterForbidden)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.ProblemCodeSignupClosed, refused.Code.Value)
	exists, err := env.DB.User.Query().Where(entuser.Email("grace@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestAClosedInstanceStillAcceptsAnInvitation(t *testing.T) {
	env := closedInstance(t)
	token := inviteToAcme(t, env, "newcomer@example.com")

	res, err := env.SiteAnonymous(t).SitePublicInvitationsAccept(t.Context(),
		&siteapi.SiteAcceptInvitationInput{Name: siteapi.NewOptString("Newcomer"), Password: siteapi.NewOptString("newcomer-password-1")},
		siteapi.SitePublicInvitationsAcceptParams{Token: token})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SitePublicInvitationsAcceptOK{}, res)

	exists, err := env.DB.User.Query().Where(entuser.Email("newcomer@example.com")).Exist(t.Context())
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestTheFirstOwnerOfAClosedInstanceComesFromCreateOwnerAndSignsIn(t *testing.T) {
	env := closedInstance(t)

	password, err := accounts.New(env.DB, env.Bus).CreateOwner(t.Context(), "Ada Lovelace", "ada@example.com")
	require.NoError(t, err)

	rec := postLogin(t, env, "ada@example.com", password)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotNil(t, sessionCookie(rec.Result()))
	var body siteapi.SiteLoginResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, siteapi.SiteLoginOutcomeSession, body.Outcome)
}
