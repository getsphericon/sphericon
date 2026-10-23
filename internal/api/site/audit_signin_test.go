package site_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// diffOf is the entry's diff as JSON text.
func diffOf(t *testing.T, e siteapi.SiteAuditEntryResource) string {
	t.Helper()
	diff, err := json.Marshal(e.Diff.Value)
	require.NoError(t, err)
	return string(diff)
}

// entriesInMarysWorkspaces are the entries of one action in the logs of Acme and Globex,
// the two Workspaces Mary belongs to, read by each one's Owner.
func entriesInMarysWorkspaces(t *testing.T, env *testhelper.TestEnv, action string) (acme, globex []siteapi.SiteAuditEntryResource) {
	t.Helper()
	return entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, action),
		entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, action)
}

func TestAuditProviderLoginNamesTheProviderInEachOfTheUsersWorkspaces(t *testing.T) {
	env, fake := signinEnv(t)
	signedIn(t, env, fake, mary)

	acme, globex := entriesInMarysWorkspaces(t, env, events.ActionUserLogin)

	require.Len(t, acme, 1)
	require.Len(t, globex, 1)
	for _, e := range []siteapi.SiteAuditEntryResource{acme[0], globex[0]} {
		assert.Equal(t, siteapi.SiteAuditActorKindUser, e.Actor.Kind)
		assert.Equal(t, "Mary", e.Actor.Name.Value)
		assert.JSONEq(t, `{"method":"google"}`, diffOf(t, e))
		assert.NotContains(t, diffOf(t, e), fixtures.MaryGoogleIdentitySubject, "the raw subject is never recorded")
	}
}

func TestAuditPasswordLoginNamesThePasswordAsTheMethod(t *testing.T) {
	env := testhelper.Setup(t)
	require.Equal(t, http.StatusOK, loginStatus(t, env, fixtures.MemberMaryEmail, fixtures.MemberMaryPassword))

	acme, _ := entriesInMarysWorkspaces(t, env, events.ActionUserLogin)

	require.Len(t, acme, 1)
	assert.JSONEq(t, `{"method":"password"}`, diffOf(t, acme[0]))
}

func TestAuditProviderLoginBehindASecondFactorIsRecordedWithTheProvider(t *testing.T) {
	env, fake, clock := signinEnvOnClock(t)
	resp := signInWith(t, fake, env.NewRedirectBrowser(t), samIdentity, "")
	challenge := challengeOf(t, resp)
	require.Empty(t, entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionUserLogin), "the challenge alone is not a login")

	require.Equal(t, http.StatusOK, secondStepStatus(t, env, challenge, samTOTP(t, clock)).Code)

	got := entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, events.ActionUserLogin)
	require.Len(t, got, 1)
	assert.JSONEq(t, `{"method":"google"}`, diffOf(t, got[0]))
}

func TestAuditLinkAndUnlinkNameTheProviderInEachOfTheUsersWorkspaces(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, mary)

	link(t, fake, b, maryAtWork)

	acme, globex := entriesInMarysWorkspaces(t, env, events.ActionUserIdentityLink)
	require.Len(t, acme, 1)
	require.Len(t, globex, 1)
	for _, e := range []siteapi.SiteAuditEntryResource{acme[0], globex[0]} {
		assert.Equal(t, "Mary", e.Actor.Name.Value)
		assert.JSONEq(t, `{"provider":"google"}`, diffOf(t, e))
		assert.NotContains(t, diffOf(t, e), maryAtWork.Subject, "the raw subject is never recorded")
		assert.NotContains(t, diffOf(t, e), maryAtWork.Email)
	}

	list := listIdentities(t, b)
	require.Len(t, list.Items, 2)
	require.Equal(t, http.StatusNoContent, b.Delete(t, identitiesPath+"/"+list.Items[0].ID).StatusCode)

	acme, globex = entriesInMarysWorkspaces(t, env, events.ActionUserIdentityUnlink)
	require.Len(t, acme, 1)
	require.Len(t, globex, 1)
	assert.JSONEq(t, `{"provider":"google"}`, diffOf(t, acme[0]))
}

func TestAuditARefusedLinkOrUnlinkRecordsNothing(t *testing.T) {
	env, fake := signinEnv(t)
	b := signedIn(t, env, fake, newcomer)
	list := listIdentities(t, b)
	require.Len(t, list.Items, 1)

	require.Equal(t, http.StatusConflict, b.Delete(t, identitiesPath+"/"+list.Items[0].ID).StatusCode, "the last way to sign in")

	for _, action := range []string{events.ActionUserIdentityLink, events.ActionUserIdentityUnlink} {
		for _, ev := range env.OutboxEvents(t, events.NameAuditEntry) {
			assert.NotEqual(t, action, ev.(*events.AuditEntry).Action)
		}
	}
}
