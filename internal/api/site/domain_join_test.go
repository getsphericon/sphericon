package site_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

const joinerPassword = "joiner-password-1"

// setDomainJoin switches Acme's domain join as its Owner and returns the answer.
func setDomainJoin(t *testing.T, c *siteapi.Client, enabled bool) siteapi.SiteDomainJoinSetRes {
	t.Helper()
	res, err := c.SiteDomainJoinSet(context.Background(), &siteapi.SiteSetDomainJoinInput{Enabled: enabled},
		siteapi.SiteDomainJoinSetParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	return res
}

// enableAcmeJoin has Acme's Owner switch domain join on; acme.com is already verified there.
func enableAcmeJoin(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	require.IsType(t, &siteapi.SiteDomainJoinResource{}, setDomainJoin(t, env.SiteActor(t, fixtures.OwnerJohnEmail), true))
}

// signedUp is a person who signed up and followed the verify link: the client holds their
// session, landing is the Workspace the link answered with.
type signedUp struct {
	client  *siteapi.Client
	landing siteapi.WorkspaceId
}

// signUp takes a new person through Sign-up and the verify link.
func signUp(t *testing.T, env *testhelper.TestEnv, email string) signedUp {
	t.Helper()
	res := verify(t, env, signUpLink(t, env, "New Colleague", email), joinerPassword)
	ok, isOK := res.(*siteapi.SiteVerifyEmailResultHeaders)
	require.Truef(t, isOK, "got %T", res)
	cookie, err := http.ParseSetCookie(ok.SetCookie)
	require.NoError(t, err)
	return signedUp{client: env.SiteWithToken(t, cookie.Value), landing: ok.Response.LandingWorkspaceId}
}

func (n signedUp) workspaces(t *testing.T) []siteapi.SiteWorkspaceResource {
	t.Helper()
	spaces, err := n.client.SiteWorkspacesList(context.Background())
	require.NoError(t, err)
	return spaces
}

func (n signedUp) workspaceNames(t *testing.T) []string {
	t.Helper()
	return lo.Map(n.workspaces(t), func(w siteapi.SiteWorkspaceResource, _ int) string { return w.Name })
}

func acmeMembers(t *testing.T, owner *siteapi.Client) []siteapi.SiteMembershipResource {
	t.Helper()
	res, err := owner.SiteMembershipsList(context.Background(), siteapi.SiteMembershipsListParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteMembershipsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	return *list
}

func acmeMembersNamed(members []siteapi.SiteMembershipResource, email string) []siteapi.SiteMembershipResource {
	return lo.Filter(members, func(m siteapi.SiteMembershipResource, _ int) bool { return string(m.Email) == email })
}

func TestTheVerifyLinkJoinsTheWorkspaceOfTheAddressDomainAndLandsThere(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)

	joiner := signUp(t, env, "newbie@acme.com")

	assert.Equal(t, siteapi.WorkspaceId(fixtures.AcmePublicID), joiner.landing)
	joined := acmeMembersNamed(acmeMembers(t, env.SiteActor(t, fixtures.OwnerJohnEmail)), "newbie@acme.com")
	require.Len(t, joined, 1)
	assert.Equal(t, siteapi.SiteMembershipRoleMember, joined[0].Role)
	assert.Equal(t, siteapi.NewOptSiteMembershipSource(siteapi.SiteMembershipSourceDomain), joined[0].Source)
}

func TestAJoinerStillGetsAPersonalWorkspaceOfTheirOwn(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)

	joiner := signUp(t, env, "newbie@acme.com")

	assert.ElementsMatch(t, []string{fixtures.AcmeName, "Personal"}, joiner.workspaceNames(t))
}

func TestTheAddressDomainIsMatchedInAnyCase(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)

	joiner := signUp(t, env, "Newbie@ACME.com")

	assert.Equal(t, siteapi.WorkspaceId(fixtures.AcmePublicID), joiner.landing)
}

func TestWithoutAJoinAThePersonalWorkspaceIsTheLanding(t *testing.T) {
	env := testhelper.Setup(t)

	outsider := signUp(t, env, "someone@elsewhere.example")

	spaces := outsider.workspaces(t)
	require.Len(t, spaces, 1)
	assert.Equal(t, spaces[0].ID, outsider.landing)
}

func TestDomainJoinIsOffByDefaultSoAVerifiedDomainAdmitsNobody(t *testing.T) {
	env := testhelper.Setup(t)

	joiner := signUp(t, env, "newbie@acme.com")

	assert.Equal(t, []string{"Personal"}, joiner.workspaceNames(t))
	assert.Empty(t, acmeMembersNamed(acmeMembers(t, env.SiteActor(t, fixtures.OwnerJohnEmail)), "newbie@acme.com"))
}

func TestSwitchingJoinOffStopsNewJoinsAndRemovesNobody(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	enableAcmeJoin(t, env)
	early := signUp(t, env, "early@acme.com")

	require.IsType(t, &siteapi.SiteDomainJoinResource{}, setDomainJoin(t, owner, false))
	late := signUp(t, env, "late@acme.com")

	assert.Equal(t, siteapi.WorkspaceId(fixtures.AcmePublicID), early.landing)
	assert.Equal(t, []string{"Personal"}, late.workspaceNames(t))
	members := acmeMembers(t, owner)
	assert.Len(t, acmeMembersNamed(members, "early@acme.com"), 1)
	assert.Empty(t, acmeMembersNamed(members, "late@acme.com"))
}

// A person whose address was proven while the switch was off is not added when it is
// turned on later: only the moment of proof joins.
func TestAnAddressProvenBeforeTheSwitchWasTurnedOnIsNotAdded(t *testing.T) {
	env := testhelper.Setup(t)
	earlier := signUp(t, env, "earlier@acme.com")

	enableAcmeJoin(t, env)

	assert.Equal(t, []string{"Personal"}, earlier.workspaceNames(t))
	assert.Empty(t, acmeMembersNamed(acmeMembers(t, env.SiteActor(t, fixtures.OwnerJohnEmail)), "earlier@acme.com"))
}

func TestAPendingJoinDomainAdmitsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)

	joiner := signUp(t, env, "newbie@"+fixtures.JoinDomainPendingDomain)

	assert.Equal(t, []string{"Personal"}, joiner.workspaceNames(t))
}

// A verified domain does not cover its subdomains, nor does a verified subdomain cover its parent.
func TestASubdomainOfAVerifiedJoinDomainDoesNotJoin(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)

	joiner := signUp(t, env, "newbie@mail.acme.com")

	assert.Equal(t, []string{"Personal"}, joiner.workspaceNames(t))
}

func TestARemovedJoinDomainAdmitsNobodyAndKeepsItsMembers(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	enableAcmeJoin(t, env)
	early := signUp(t, env, "early@acme.com")

	res, err := owner.SiteJoinDomainsRemove(context.Background(),
		siteapi.SiteJoinDomainsRemoveParams{WorkspaceId: fixtures.AcmePublicID, ID: siteapi.EntityId(fixtures.JoinDomainVerifiedID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteJoinDomainsRemoveNoContent{}, res)
	late := signUp(t, env, "late@acme.com")

	assert.Contains(t, early.workspaceNames(t), fixtures.AcmeName)
	assert.Equal(t, []string{"Personal"}, late.workspaceNames(t))
}

// A domain that moved to another Workspace is no longer the old one's: its record was gone,
// the other Workspace proved it, and the next person joins the new holder only.
func TestADomainHandedOverJoinsItsNewHolderOnly(t *testing.T) {
	zone := dnstest.NewZone(t)
	env := testhelper.Setup(t, testhelper.WithTXTLookup(zone.LookupTXT))
	enableAcmeJoin(t, env)
	globex := env.SiteActor(t, fixtures.OwnerJaneEmail)
	require.IsType(t, &siteapi.SiteDomainJoinResource{}, setGlobexJoin(t, globex))
	added := mustAddJoinDomain(t, globex, fixtures.GlobexPublicID, fixtures.JoinDomainVerifiedDomain)
	zone.PublishTXT(t, added.Record.Host, added.Record.Value)
	require.IsType(t, &siteapi.SiteJoinDomainResource{}, verifyJoinDomain(t, globex, fixtures.GlobexPublicID, added.ID))

	joiner := signUp(t, env, "newbie@acme.com")

	assert.Equal(t, siteapi.WorkspaceId(fixtures.GlobexPublicID), joiner.landing)
	assert.Empty(t, acmeMembersNamed(acmeMembers(t, env.SiteActor(t, fixtures.OwnerJohnEmail)), "newbie@acme.com"))
}

func setGlobexJoin(t *testing.T, c *siteapi.Client) siteapi.SiteDomainJoinSetRes {
	t.Helper()
	res, err := c.SiteDomainJoinSet(context.Background(), &siteapi.SiteSetDomainJoinInput{Enabled: true},
		siteapi.SiteDomainJoinSetParams{WorkspaceId: fixtures.GlobexPublicID})
	require.NoError(t, err)
	return res
}

func TestASuspendedWorkspaceAdmitsNobodyEvenWithTheSwitchOnAndTheDomainVerified(t *testing.T) {
	env := testhelper.Setup(t)

	joiner := signUp(t, env, "newbie@"+fixtures.JoinDomainHooliDomain)

	assert.Equal(t, []string{"Personal"}, joiner.workspaceNames(t))
}

func TestAWorkspaceInItsDeletionWindowAdmitsNobody(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	enableAcmeJoin(t, env)
	deleted, err := owner.SiteWorkspacesDelete(context.Background(), &siteapi.SiteDeleteWorkspaceInput{Name: fixtures.AcmeName},
		siteapi.SiteWorkspacesDeleteParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteDeletedWorkspaceResource{}, deleted)

	joiner := signUp(t, env, "newbie@acme.com")

	assert.Equal(t, []string{"Personal"}, joiner.workspaceNames(t))
}

func TestTheWorkspacesTwoFactorRequirementAppliesToAJoinerWithTheUsualGrace(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	enableAcmeJoin(t, env)
	required, err := owner.SiteWorkspacesSetSecondFactorRequirement(context.Background(),
		&siteapi.SiteSecondFactorRequirementInput{Required: true},
		siteapi.SiteWorkspacesSetSecondFactorRequirementParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteWorkspaceResource{}, required)

	joiner := signUp(t, env, "newbie@acme.com")

	acme := lo.Filter(joiner.workspaces(t), func(w siteapi.SiteWorkspaceResource, _ int) bool { return w.ID == fixtures.AcmePublicID })
	require.Len(t, acme, 1)
	require.True(t, acme[0].SecondFactorGraceEndsAt.Set)
	assert.WithinDuration(t, time.Now().Add(7*24*time.Hour), time.Time(acme[0].SecondFactorGraceEndsAt.Value), time.Minute)
}

func TestAJoinSendsNoMailToTheJoinerOrTheOwnerBeyondTheSignUpAndWelcome(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)
	before := len(env.SystemMail.Messages())

	signUp(t, env, "newbie@acme.com")

	sent := env.SystemMail.Messages()[before:]
	recipients := lo.Map(sent, func(m messaging.EmailMessage, _ int) string { return m.To })
	assert.NotContains(t, recipients, fixtures.OwnerJohnEmail)
	assert.Len(t, sent, 2, "the verification email and the welcome email, nothing about the join")
}

func TestAJoinIsAuditedWithTheSystemAsActorTheJoinerAsTargetAndTheDomain(t *testing.T) {
	env := testhelper.Setup(t)
	enableAcmeJoin(t, env)

	signUp(t, env, "newbie@acme.com")

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionMembershipDomainJoin)
	require.Len(t, got, 1)
	assert.Equal(t, "system", string(got[0].Actor.Kind))
	assert.Equal(t, "user", got[0].Target.Type)
	assert.Equal(t, "New Colleague", got[0].Target.Name.Value)
	diff, err := json.Marshal(got[0].Diff.Value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"domain": {"to": "acme.com"}, "role": {"to": "member"}}`, string(diff))
}

func TestSwitchingDomainJoinIsAuditedAsAWorkspaceUpdate(t *testing.T) {
	env := testhelper.Setup(t)

	enableAcmeJoin(t, env)

	got := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, events.ActionWorkspaceUpdate)
	require.Len(t, got, 1)
	diff, err := json.Marshal(got[0].Diff.Value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"domain_join_enabled": {"from": false, "to": true}}`, string(diff))
}

func TestAnAdminReadsAndSwitchesDomainJoin(t *testing.T) {
	env := testhelper.Setup(t)
	admin := env.SiteActor(t, fixtures.AdminAliceEmail)

	assert.Equal(t, &siteapi.SiteDomainJoinResource{Enabled: true}, setDomainJoin(t, admin, true))
	read, err := admin.SiteDomainJoinGet(context.Background(), siteapi.SiteDomainJoinGetParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.Equal(t, &siteapi.SiteDomainJoinResource{Enabled: true}, read)
}

func TestAMemberCannotReadOrSwitchDomainJoin(t *testing.T) {
	env := testhelper.Setup(t)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	read, err := c.SiteDomainJoinGet(context.Background(), siteapi.SiteDomainJoinGetParams{WorkspaceId: fixtures.AcmePublicID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteDomainJoinGetForbidden{}, read)
	assert.IsType(t, &siteapi.SiteDomainJoinSetForbidden{}, setDomainJoin(t, c, true))
}

func TestAnOwnerReadsDomainJoin(t *testing.T) {
	env := testhelper.Setup(t)

	read, err := env.SiteActor(t, fixtures.OwnerJohnEmail).SiteDomainJoinGet(context.Background(), siteapi.SiteDomainJoinGetParams{WorkspaceId: fixtures.AcmePublicID})

	require.NoError(t, err)
	assert.Equal(t, &siteapi.SiteDomainJoinResource{Enabled: false}, read)
}

func TestTheSwitchIsReadBackAsSet(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	enableAcmeJoin(t, env)

	read, err := owner.SiteDomainJoinGet(context.Background(), siteapi.SiteDomainJoinGetParams{WorkspaceId: fixtures.AcmePublicID})

	require.NoError(t, err)
	assert.Equal(t, &siteapi.SiteDomainJoinResource{Enabled: true}, read)
}
