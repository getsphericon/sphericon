package site_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gptoken "github.com/go-pkgz/auth/v2/token"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// Umbrella's requirement started 2026-03-01: Rita's Membership predates it (grace
// ends 2026-03-08), Nina joined 2026-03-05 (grace ends 2026-03-12). Fixture times
// are local times.
var (
	ritaGraceEnds = time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local).Add(7 * 24 * time.Hour)
	ninaGraceEnds = time.Date(2026, 3, 5, 0, 0, 0, 0, time.Local).Add(7 * 24 * time.Hour)
)

type requirementClock struct{ t time.Time }

func (c *requirementClock) now() time.Time { return c.t }

func requirementEnv(t *testing.T, at time.Time) *testhelper.TestEnv {
	t.Helper()
	c := &requirementClock{t: at}
	return testhelper.Setup(t, testhelper.WithClock(c.now))
}

func setRequirement(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId, on bool) siteapi.SiteWorkspacesSetSecondFactorRequirementRes {
	t.Helper()
	res, err := c.SiteWorkspacesSetSecondFactorRequirement(t.Context(),
		&siteapi.SiteSecondFactorRequirementInput{Required: on},
		siteapi.SiteWorkspacesSetSecondFactorRequirementParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

func workspaceOf(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) siteapi.SiteWorkspaceResource {
	t.Helper()
	list, err := c.SiteWorkspacesList(t.Context())
	require.NoError(t, err)
	for i := range list {
		if list[i].ID == workspaceID {
			return list[i]
		}
	}
	require.Failf(t, "workspace not listed", "%s", workspaceID)
	return siteapi.SiteWorkspaceResource{}
}

// siteGet sends a GET as the fixture user and returns the status and the body.
func siteGet(t *testing.T, env *testhelper.TestEnv, email, path string) (status int, body []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	token := env.SiteToken(t, email, nil)
	req.AddCookie(testhelper.RequestCookie("JWT", token))
	env.Server.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// problemCode is the `code` of an RFC 7807 problem body.
func problemCode(t *testing.T, body []byte) string {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(body, &problem))
	return problem.Code
}

func TestOwnerAndAdminSwitchTheRequirementAndAMemberCannot(t *testing.T) {
	at := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	c := &requirementClock{t: at}
	env := testhelper.Setup(t, testhelper.WithClock(c.now))
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := setRequirement(t, john, fixtures.AcmePublicID, true)
	require.IsType(t, &siteapi.SiteWorkspaceResource{}, res)
	w := res.(*siteapi.SiteWorkspaceResource)
	assert.True(t, time.Time(w.SecondFactorRequiredAt.Value).Equal(at))
	assert.True(t, time.Time(w.SecondFactorGraceEndsAt.Value).Equal(at.Add(7*24*time.Hour)),
		"an existing Membership gets 7 days from the requirement's start")

	entries := env.OutboxEvents(t, events.NameAuditEntry)
	require.Len(t, entries, 1)
	e := entries[0].(*events.AuditEntry)
	assert.Equal(t, events.ActionWorkspaceUpdate, e.Action)
	assert.Contains(t, e.Diff, "second_factor_required_at")

	// Switching it on again keeps the start, so no one's grace restarts.
	c.t = at.Add(48 * time.Hour)
	again := setRequirement(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID, true).(*siteapi.SiteWorkspaceResource)
	assert.True(t, time.Time(again.SecondFactorRequiredAt.Value).Equal(at))
	assert.Len(t, env.OutboxEvents(t, events.NameAuditEntry), 1)

	mary := env.SiteActor(t, fixtures.MemberMaryEmail)
	assert.IsType(t, &siteapi.SiteWorkspacesSetSecondFactorRequirementForbidden{}, setRequirement(t, mary, fixtures.AcmePublicID, false))
	oscar := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	assert.IsType(t, &siteapi.SiteWorkspacesSetSecondFactorRequirementNotFound{}, setRequirement(t, oscar, fixtures.AcmePublicID, false))

	sam := env.SiteActor(t, fixtures.SecondFactorSamEmail)
	cleared := setRequirement(t, sam, fixtures.UmbrellaPublicID, false)
	require.IsType(t, &siteapi.SiteWorkspaceResource{}, cleared, "an admin may clear it")
	assert.False(t, cleared.(*siteapi.SiteWorkspaceResource).SecondFactorRequiredAt.Set)
	assert.Len(t, env.OutboxEvents(t, events.NameAuditEntry), 2)
}

func TestGraceEndsSevenDaysAfterTheLaterOfTheRequirementAndTheMembership(t *testing.T) {
	env := requirementEnv(t, time.Date(2026, 3, 4, 0, 0, 0, 0, time.Local))

	rita := workspaceOf(t, env.SiteActor(t, fixtures.UmbrellaOwnerRitaEmail), fixtures.UmbrellaPublicID)
	assert.True(t, time.Time(rita.SecondFactorGraceEndsAt.Value).Equal(ritaGraceEnds), "existing Membership")

	nina := workspaceOf(t, env.SiteActor(t, fixtures.UmbrellaMemberNinaEmail), fixtures.UmbrellaPublicID)
	assert.True(t, time.Time(nina.SecondFactorGraceEndsAt.Value).Equal(ninaGraceEnds), "new Membership")

	sam := workspaceOf(t, env.SiteActor(t, fixtures.SecondFactorSamEmail), fixtures.UmbrellaPublicID)
	assert.True(t, sam.SecondFactorRequiredAt.Set)
	assert.False(t, sam.SecondFactorGraceEndsAt.Set, "a User with a Second factor has no deadline")

	assert.Equal(t, siteapi.SiteMembershipRoleOwner, rita.Role, "the list carries the User's role")
	assert.Equal(t, siteapi.SiteMembershipRoleMember, nina.Role)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, sam.Role)

	initech := workspaceOf(t, env.SiteActor(t, fixtures.UmbrellaOwnerRitaEmail), fixtures.InitechPublicID)
	assert.False(t, initech.SecondFactorGraceEndsAt.Set, "a Workspace without the requirement")

	code, _ := siteGet(t, env, fixtures.UmbrellaOwnerRitaEmail, "/site/workspaces/"+fixtures.UmbrellaPublicID+"/tags")
	assert.Equal(t, http.StatusOK, code, "in grace the Workspace is reachable")
}

func TestAfterGraceTheRequiringWorkspaceAloneIsWithheld(t *testing.T) {
	env := requirementEnv(t, ritaGraceEnds.Add(time.Hour))

	code, body := siteGet(t, env, fixtures.UmbrellaOwnerRitaEmail, "/site/workspaces/"+fixtures.UmbrellaPublicID+"/tags")
	assert.Equal(t, http.StatusForbidden, code)
	problem := problemCode(t, body)
	assert.Equal(t, "second_factor_required", problem)

	for _, path := range []string{
		"/site/workspaces/" + fixtures.InitechPublicID + "/tags",
		"/site/workspaces",
		"/site/me",
		"/site/me/second-factor",
	} {
		code, _ := siteGet(t, env, fixtures.UmbrellaOwnerRitaEmail, path)
		assert.Equal(t, http.StatusOK, code, path)
	}

	code, _ = siteGet(t, env, fixtures.UmbrellaMemberNinaEmail, "/site/workspaces/"+fixtures.UmbrellaPublicID+"/tags")
	assert.Equal(t, http.StatusOK, code, "Nina's grace runs from her later Membership")

	code, _ = siteGet(t, env, fixtures.SecondFactorSamEmail, "/site/workspaces/"+fixtures.UmbrellaPublicID+"/tags")
	assert.Equal(t, http.StatusOK, code, "one Second factor satisfies every Workspace")
}

func TestANewMembershipIsWithheldOnceItsOwnGraceEnds(t *testing.T) {
	env := requirementEnv(t, ninaGraceEnds.Add(time.Hour))
	code, body := siteGet(t, env, fixtures.UmbrellaMemberNinaEmail, "/site/workspaces/"+fixtures.UmbrellaPublicID+"/tags")
	assert.Equal(t, http.StatusForbidden, code)
	problem := problemCode(t, body)
	assert.Equal(t, "second_factor_required", problem)
}

func TestTurningTheRequirementOnEmailsEveryUserWithoutASecondFactor(t *testing.T) {
	env := requirementEnv(t, time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC))
	sam := env.SiteActor(t, fixtures.SecondFactorSamEmail)

	setRequirement(t, sam, fixtures.UmbrellaPublicID, false)
	assert.Empty(t, env.SystemMail.Messages(), "switching it off mails no one")

	setRequirement(t, sam, fixtures.UmbrellaPublicID, true)
	msgs := env.SystemMail.Messages()
	to := make([]string, 0, len(msgs))
	for _, m := range msgs {
		to = append(to, m.To)
		assert.Contains(t, m.Subject, "Umbrella")
		assert.Contains(t, m.Text, "2026-04-08 12:00 UTC", "the deadline is 7 days after the requirement's start")
		assert.Contains(t, m.Text, "/account/security")
	}
	assert.ElementsMatch(t, []string{fixtures.UmbrellaOwnerRitaEmail, fixtures.UmbrellaMemberNinaEmail}, to,
		"Users without a Second factor; Sam has one")

	setRequirement(t, sam, fixtures.UmbrellaPublicID, true)
	assert.Len(t, env.SystemMail.Messages(), 2, "switching it on while it is on mails no one")
}

// Story 35: an Owner or Admin withheld by the requirement can still turn it off;
// every other operation of the Workspace stays withheld, and the role check holds.
func TestAWithheldOwnerCanStillTurnTheRequirementOff(t *testing.T) {
	env := requirementEnv(t, ninaGraceEnds.Add(time.Hour))

	nina := setRequirement(t, env.SiteActor(t, fixtures.UmbrellaMemberNinaEmail), fixtures.UmbrellaPublicID, false)
	assert.IsType(t, &siteapi.SiteWorkspacesSetSecondFactorRequirementForbidden{}, nina, "a withheld member still may not")

	code, _ := siteGet(t, env, fixtures.UmbrellaOwnerRitaEmail, "/site/workspaces/"+fixtures.UmbrellaPublicID+"/tags")
	require.Equal(t, http.StatusForbidden, code, "Rita is withheld")

	res := setRequirement(t, env.SiteActor(t, fixtures.UmbrellaOwnerRitaEmail), fixtures.UmbrellaPublicID, false)
	require.IsType(t, &siteapi.SiteWorkspaceResource{}, res)
	assert.False(t, res.(*siteapi.SiteWorkspaceResource).SecondFactorRequiredAt.Set)

	code, _ = siteGet(t, env, fixtures.UmbrellaOwnerRitaEmail, "/site/workspaces/"+fixtures.UmbrellaPublicID+"/tags")
	assert.Equal(t, http.StatusOK, code, "without the requirement Umbrella is reachable again")
}

// The exemption is for turning the requirement off only.
func TestAWithheldOwnerCannotSwitchTheRequirementOn(t *testing.T) {
	env := requirementEnv(t, ritaGraceEnds.Add(time.Hour))
	res := setRequirement(t, env.SiteActor(t, fixtures.UmbrellaOwnerRitaEmail), fixtures.UmbrellaPublicID, true)
	require.IsType(t, &siteapi.SiteWorkspacesSetSecondFactorRequirementForbidden{}, res)
	assert.Equal(t, siteapi.ProblemCodeSecondFactorRequired,
		res.(*siteapi.SiteWorkspacesSetSecondFactorRequirementForbidden).Code.Value)
}

func TestOneSecondFactorSatisfiesEveryRequiringWorkspace(t *testing.T) {
	c := &requirementClock{t: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)}
	env := testhelper.Setup(t, testhelper.WithClock(c.now))
	require.IsType(t, &siteapi.SiteWorkspaceResource{},
		setRequirement(t, env.SiteActor(t, fixtures.OwnerJaneEmail), fixtures.GlobexPublicID, true))
	c.t = c.t.Add(8 * 24 * time.Hour)

	for _, path := range []string{"/site/workspaces/" + fixtures.GlobexPublicID + "/tags", "/site/workspaces/" + fixtures.UmbrellaPublicID + "/tags"} {
		code, _ := siteGet(t, env, fixtures.SecondFactorSamEmail, path)
		assert.Equal(t, http.StatusOK, code, path)
	}
	code, _ := siteGet(t, env, fixtures.OwnerJaneEmail, "/site/workspaces/"+fixtures.GlobexPublicID+"/tags")
	assert.Equal(t, http.StatusForbidden, code, "Jane has no Second factor: Globex is withheld from her")
}

func TestAStrongSessionMeetsTheRequirementWithoutASecondFactor(t *testing.T) {
	env := requirementEnv(t, ritaGraceEnds.Add(time.Hour))
	path := "/site/workspaces/" + fixtures.UmbrellaPublicID + "/tags"
	get := func(method apiauth.Method) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		token := env.SiteToken(t, fixtures.UmbrellaOwnerRitaEmail, func(c *gptoken.Claims) {
			c.User.SetStrAttr(apiauth.ClaimMethod, string(method))
		})
		req.AddCookie(testhelper.RequestCookie("JWT", token))
		env.Server.ServeHTTP(rec, req)
		return rec.Code
	}

	assert.Equal(t, http.StatusForbidden, get(apiauth.MethodPassword))
	assert.Equal(t, http.StatusForbidden, get(apiauth.MethodProvider))
	assert.Equal(t, http.StatusOK, get(apiauth.MethodSecondFactor))
	assert.Equal(t, http.StatusOK, get(apiauth.MethodPasskey))
}

// The grace end the Workspace list shows follows the withhold decision: a Passkey session
// of a User with no Second factor meets the requirement, a password or provider one does not.
func TestTheGraceEndFollowsTheStrengthOfTheSession(t *testing.T) {
	c := &requirementClock{t: time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)}
	env := testhelper.Setup(t, testhelper.WithClock(c.now))
	require.IsType(t, &siteapi.SiteWorkspaceResource{},
		setRequirement(t, env.SiteActor(t, fixtures.OwnerJaneEmail), fixtures.GlobexPublicID, true))
	graceEnds := func(method apiauth.Method) any {
		rec := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/site/workspaces", nil)
		token := env.SiteToken(t, fixtures.PasskeyPatEmail, func(c *gptoken.Claims) {
			c.User.SetStrAttr(apiauth.ClaimMethod, string(method))
		})
		req.AddCookie(testhelper.RequestCookie("JWT", token))
		env.Server.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var list []map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		require.Len(t, list, 1)
		return list[0]["secondFactorGraceEndsAt"]
	}

	assert.NotNil(t, graceEnds(apiauth.MethodPassword))
	assert.NotNil(t, graceEnds(apiauth.MethodProvider))
	assert.Nil(t, graceEnds(apiauth.MethodPasskey), "a Passkey session has no grace to count down")
	assert.Nil(t, graceEnds(apiauth.MethodSecondFactor))
}
