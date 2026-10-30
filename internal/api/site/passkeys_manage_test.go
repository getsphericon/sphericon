package site_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	gptoken "github.com/go-pkgz/auth/v2/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func passkeyPath(id int64) string { return passkeysPath + "/" + strconv.FormatInt(id, 10) }

func removeBody(password string) string {
	if password == "" {
		return `{}`
	}
	return `{"currentPassword":"` + password + `"}`
}

func TestAUserRenamesAPasskeyAndSeesTheNewNameListed(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)

	rec := passkeyCall(t, env, http.MethodPatch, passkeyPath(fixtures.PasskeyPatLaptopID), session, "", `{"name":"  Pat's desk  "}`)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"Pat's desk"}, names(listPasskeys(t, env, session).Items))
}

func TestRenamingAPasskeyNeedsAName(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)

	empty := passkeyCall(t, env, http.MethodPatch, passkeyPath(fixtures.PasskeyPatLaptopID), session, "", `{"name":"   "}`)

	assert.Equal(t, http.StatusUnprocessableEntity, empty.Code)
	assert.Equal(t, []string{fixtures.PasskeyPatLaptopName}, names(listPasskeys(t, env, session).Items))
}

func TestAnotherUsersPasskeyCannotBeRenamedOrRemoved(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)

	renamed := passkeyCall(t, env, http.MethodPatch, passkeyPath(fixtures.PasskeyPegKeyID), session, "", `{"name":"mine now"}`)
	removed := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPegKeyID), session, "", removeBody(fixtures.PasskeyPatPassword))

	assert.Equal(t, http.StatusNotFound, renamed.Code)
	assert.Equal(t, http.StatusNotFound, removed.Code)
	peg := env.SiteToken(t, fixtures.PasskeyOnlyPegEmail, nil)
	assert.Equal(t, []string{fixtures.PasskeyPegKeyName}, names(listPasskeys(t, env, peg).Items))
}

func TestAUserRemovesAPasskeyWithThePasswordAndOtherSessionsEnd(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)
	other := patSession(t, env)

	rec := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPatLaptopID), session, "", removeBody(fixtures.PasskeyPatPassword))

	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	fresh := reissued(t, rec)
	require.NotEmpty(t, fresh, "the acting session is reissued")
	assert.Empty(t, listPasskeys(t, env, fresh).Items)
	assert.Equal(t, http.StatusUnauthorized, workspacesStatus(t, env, other), "another session ended")
}

func TestARemovedPasskeyIsNoLongerExcludedFromANewRegistration(t *testing.T) {
	env := testhelper.Setup(t)
	removed := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPatLaptopID), patSession(t, env), "", removeBody(fixtures.PasskeyPatPassword))
	require.Equal(t, http.StatusNoContent, removed.Code, removed.Body.String())

	options, _ := begin(t, env, reissued(t, removed))

	var parsed struct {
		ExcludeCredentials []json.RawMessage `json:"excludeCredentials"`
	}
	require.NoError(t, json.Unmarshal(options, &parsed))
	assert.Empty(t, parsed.ExcludeCredentials)
}

func TestRemovingAPasskeyMailsTheOwner(t *testing.T) {
	env := testhelper.Setup(t)

	rec := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPatLaptopID), patSession(t, env), "", removeBody(fixtures.PasskeyPatPassword))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	sent := env.SystemMail.Messages()
	require.Len(t, sent, 1)
	assert.Equal(t, fixtures.PasskeyPatEmail, sent[0].To)
	assert.Contains(t, sent[0].Subject, "passkey was removed")
	assert.Contains(t, sent[0].Text, fixtures.PasskeyPatLaptopName)
}

func TestRemovingAPasskeyIsRefusedWithoutProvingTheUser(t *testing.T) {
	env := testhelper.Setup(t)
	session := patSession(t, env)

	rec := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPatLaptopID), session, "", removeBody("not the password"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, []string{fixtures.PasskeyPatLaptopName}, names(listPasskeys(t, env, session).Items))
	assert.Empty(t, env.SystemMail.Messages())
}

func TestAUserWithNoPasswordAndNoLinkedIdentityCannotRemoveTheirLastPasskey(t *testing.T) {
	env := testhelper.Setup(t)
	session := env.SiteToken(t, fixtures.PasskeyOnlyPegEmail, func(c *gptoken.Claims) {
		c.User.SetStrAttr(apiauth.ClaimProviderLogin, strconv.FormatInt(time.Now().Unix(), 10))
	})

	rec := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPegKeyID), session, "", removeBody(""))

	assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, []string{fixtures.PasskeyPegKeyName}, names(listPasskeys(t, env, session).Items))
}

func TestAUserWithALinkedIdentityAndNoPasswordMayRemoveTheirLastPasskey(t *testing.T) {
	r := newReauthEnv(t)
	session := r.signIn(t, newcomer)
	options, ceremony := begin(t, r.env, session)
	credential := r.env.NewAuthenticator(t).Attest(options)
	added := passkeyCall(t, r.env, http.MethodPost, passkeyFinishPath, session, ceremony, finishBody(t, "", "Nina's laptop", credential))
	require.Equal(t, http.StatusOK, added.Code, added.Body.String())
	id := listPasskeys(t, r.env, reissued(t, added)).Items[0].ID
	numeric, err := strconv.ParseInt(id, 10, 64)
	require.NoError(t, err)
	session = r.signIn(t, newcomer)

	rec := passkeyCall(t, r.env, http.MethodDelete, passkeyPath(numeric), session, "", removeBody(""))

	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Empty(t, listPasskeys(t, r.env, reissued(t, rec)).Items)
}

func TestAUserWithAPasswordMayRemoveTheirOnlyPasskey(t *testing.T) {
	env := testhelper.Setup(t)

	rec := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPatLaptopID), patSession(t, env), "", removeBody(fixtures.PasskeyPatPassword))

	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

func TestAPasskeyRemovedByItsUserNoLongerSignsAnyoneIn(t *testing.T) {
	env := testhelper.Setup(t)
	options, ceremony := beginLogin(t, env)
	assertion := env.FixturePasskey(t).Assert(options)
	removed := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPatLaptopID), patSession(t, env), "", removeBody(fixtures.PasskeyPatPassword))
	require.Equal(t, http.StatusNoContent, removed.Code, removed.Body.String())

	rec := finishLogin(t, env, ceremony, assertion)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Nil(t, sessionCookie(rec.Result()))
}

// A Passkey session meets the Two-factor requirement only while the User holds a Passkey:
// removing the last one reissues the acting session as the password session it falls back to.
func TestRemovingTheLastPasskeyWeakensTheActingSessionUnderTheRequirement(t *testing.T) {
	c := &requirementClock{t: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)}
	env := testhelper.Setup(t, testhelper.WithClock(c.now))
	require.IsType(t, &siteapi.SiteWorkspaceResource{},
		requireSecondFactor(t, env.SiteActor(t, fixtures.OwnerJaneEmail), fixtures.GlobexPublicID))
	c.t = c.t.Add(8 * 24 * time.Hour)
	tags := "/site/workspaces/" + fixtures.GlobexPublicID + "/tags"
	session := env.SiteToken(t, fixtures.PasskeyPatEmail, func(claims *gptoken.Claims) {
		claims.User.SetStrAttr(apiauth.ClaimMethod, string(apiauth.MethodPasskey))
	})
	status := func(session string) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tags, nil)
		req.AddCookie(testhelper.RequestCookie("JWT", session))
		env.Server.ServeHTTP(rec, req)
		return rec.Code
	}
	require.Equal(t, http.StatusOK, status(session), "a Passkey session meets the requirement")

	rec := passkeyCall(t, env, http.MethodDelete, passkeyPath(fixtures.PasskeyPatLaptopID), session, "", removeBody(fixtures.PasskeyPatPassword))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	assert.Equal(t, http.StatusForbidden, status(reissued(t, rec)))
}
