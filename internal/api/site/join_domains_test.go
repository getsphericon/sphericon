package site_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent/joindomain"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/dnstest"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// joinDomainsEnv is a test environment whose TXT lookups are answered by a zone of the
// test's own, so a test publishes and withdraws the proof records itself.
func joinDomainsEnv(t *testing.T) (*testhelper.TestEnv, *dnstest.Zone) {
	t.Helper()
	zone := dnstest.NewZone(t)
	return testhelper.Setup(t, testhelper.WithTXTLookup(zone.LookupTXT)), zone
}

func listJoinDomains(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId) []siteapi.SiteJoinDomainResource {
	t.Helper()
	res, err := c.SiteJoinDomainsList(context.Background(), siteapi.SiteJoinDomainsListParams{WorkspaceId: ws})
	require.NoError(t, err)
	list, ok := res.(*siteapi.SiteJoinDomainsListOKApplicationJSON)
	require.Truef(t, ok, "got %T", res)
	return *list
}

func addJoinDomain(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, domain string) siteapi.SiteJoinDomainsAddRes {
	t.Helper()
	res, err := c.SiteJoinDomainsAdd(context.Background(), &siteapi.SiteAddJoinDomainInput{Domain: domain},
		siteapi.SiteJoinDomainsAddParams{WorkspaceId: ws})
	require.NoError(t, err)
	return res
}

func mustAddJoinDomain(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, domain string) *siteapi.SiteJoinDomainResource {
	t.Helper()
	res, ok := addJoinDomain(t, c, ws, domain).(*siteapi.SiteJoinDomainResource)
	require.Truef(t, ok, "adding %s answered %T", domain, res)
	return res
}

func verifyJoinDomain(t *testing.T, c *siteapi.Client, ws siteapi.WorkspaceId, id siteapi.EntityId) siteapi.SiteJoinDomainsVerifyRes {
	t.Helper()
	res, err := c.SiteJoinDomainsVerify(context.Background(), siteapi.SiteJoinDomainsVerifyParams{WorkspaceId: ws, ID: id})
	require.NoError(t, err)
	return res
}

func TestJoinDomainsListShowsVerifiedAndPendingWithTheRecordToPublish(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	got := listJoinDomains(t, owner, fixtures.AcmePublicID)

	verified, found := lo.Find(got, func(d siteapi.SiteJoinDomainResource) bool { return d.Domain == fixtures.JoinDomainVerifiedDomain })
	require.True(t, found)
	assert.True(t, verified.Verified)
	assert.True(t, verified.VerifiedAt.Set)
	pending, found := lo.Find(got, func(d siteapi.SiteJoinDomainResource) bool { return d.Domain == fixtures.JoinDomainPendingDomain })
	require.True(t, found)
	assert.False(t, pending.Verified)
	assert.Equal(t, "_sphericon-join.eu.acme.com", pending.Record.Host)
	assert.NotEmpty(t, pending.Record.Value)
	_, foreign := lo.Find(got, func(d siteapi.SiteJoinDomainResource) bool { return d.Domain == fixtures.JoinDomainGlobexDomain })
	assert.False(t, foreign, "another Workspace's claim is never listed")
}

func TestAMemberCannotManageJoinDomains(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	id := siteapi.EntityId(fixtures.JoinDomainPendingID)
	c := env.SiteActor(t, fixtures.MemberMaryEmail)

	list, err := c.SiteJoinDomainsList(ctx, siteapi.SiteJoinDomainsListParams{WorkspaceId: ws})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteJoinDomainsListForbidden{}, list)
	assert.IsType(t, &siteapi.SiteJoinDomainsAddForbidden{}, addJoinDomain(t, c, ws, "forbidden.test"))
	assert.IsType(t, &siteapi.SiteJoinDomainsVerifyForbidden{}, verifyJoinDomain(t, c, ws, id))
	remove, err := c.SiteJoinDomainsRemove(ctx, siteapi.SiteJoinDomainsRemoveParams{WorkspaceId: ws, ID: id})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteJoinDomainsRemoveForbidden{}, remove)

	stillThere := env.DB.Scoped(fixtures.AcmeID).JoinDomain().Query().Where(joindomain.ID(int64(fixtures.JoinDomainPendingID))).ExistX(ctx)
	assert.True(t, stillThere)
}

func TestJoinDomainsAddStoresTheCanonicalFormAndRefusesBadAndDuplicateDomains(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)

	added := mustAddJoinDomain(t, owner, ws, "  Bücher-Canon.TEST. ")
	assert.Equal(t, "xn--bcher-canon-thb.test", added.Domain)
	assert.False(t, added.Verified)
	assert.Equal(t, "_sphericon-join.xn--bcher-canon-thb.test", added.Record.Host)

	again := addJoinDomain(t, owner, ws, "BÜCHER-canon.test")
	assert.IsType(t, &siteapi.SiteJoinDomainsAddConflict{}, again, "the same domain in another spelling is a duplicate")
	garbage := addJoinDomain(t, owner, ws, "not a domain")
	problem, ok := garbage.(*siteapi.SiteJoinDomainsAddUnprocessableEntity)
	require.Truef(t, ok, "got %T", garbage)
	assert.Equal(t, "not a valid domain name", problem.Detail.Value, "the detail is worded, not the message id")
	assert.Equal(t, []string{"not a valid domain name"}, problem.Errors.Value["domain"])
	ip := addJoinDomain(t, owner, ws, "192.0.2.1")
	assert.IsType(t, &siteapi.SiteJoinDomainsAddUnprocessableEntity{}, ip)
	dotless := addJoinDomain(t, owner, ws, "localhost")
	assert.IsType(t, &siteapi.SiteJoinDomainsAddUnprocessableEntity{}, dotless)
}

func TestJoinDomainsPendingClaimOfAnotherWorkspaceNeverBlocks(t *testing.T) {
	env := testhelper.Setup(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)

	first := mustAddJoinDomain(t, john, fixtures.AcmePublicID, "both-claim.test")
	second := mustAddJoinDomain(t, jane, fixtures.GlobexPublicID, "both-claim.test")

	assert.NotEqual(t, first.ID, second.ID)
	assert.NotEqual(t, first.Record.Value, second.Record.Value, "each claim has its own token")
}

func TestJoinDomainsVerifyNeedsTheRecordToBePublished(t *testing.T) {
	env, zone := joinDomainsEnv(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	claim := mustAddJoinDomain(t, owner, ws, "verify-me.test")

	missing := verifyJoinDomain(t, owner, ws, claim.ID)
	assert.IsType(t, &siteapi.SiteJoinDomainsVerifyUnprocessableEntity{}, missing)

	zone.PublishTXT(t, claim.Record.Host, "another value")
	wrong := verifyJoinDomain(t, owner, ws, claim.ID)
	assert.IsType(t, &siteapi.SiteJoinDomainsVerifyUnprocessableEntity{}, wrong, "a record with another value proves nothing")

	zone.PublishTXT(t, claim.Record.Host, claim.Record.Value)
	res := verifyJoinDomain(t, owner, ws, claim.ID)
	verified, ok := res.(*siteapi.SiteJoinDomainResource)
	require.Truef(t, ok, "got %T", res)
	assert.True(t, verified.Verified)
	assert.True(t, verified.VerifiedAt.Set)
	assert.True(t, verified.LastCheckedAt.Set)

	listed := listJoinDomains(t, owner, ws)
	found, ok := lo.Find(listed, func(d siteapi.SiteJoinDomainResource) bool { return d.ID == claim.ID })
	require.True(t, ok)
	assert.True(t, found.Verified)
}

func TestJoinDomainsVerifyOfAnUnknownDomainIsNotFound(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)

	res := verifyJoinDomain(t, owner, fixtures.AcmePublicID, siteapi.EntityId(fixtures.JoinDomainGlobexID))

	assert.IsType(t, &siteapi.SiteJoinDomainsVerifyNotFound{}, res, "another Workspace's claim is not reachable")
}

func TestJoinDomainsAnotherWorkspacesVerifiedDomainIsRefusedWhileItsRecordIsPublished(t *testing.T) {
	env, zone := joinDomainsEnv(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	acme := mustAddJoinDomain(t, john, fixtures.AcmePublicID, "held.test")
	zone.PublishTXT(t, acme.Record.Host, acme.Record.Value)
	require.IsType(t, &siteapi.SiteJoinDomainResource{}, verifyJoinDomain(t, john, fixtures.AcmePublicID, acme.ID))
	globex := mustAddJoinDomain(t, jane, fixtures.GlobexPublicID, "held.test")
	zone.PublishTXT(t, globex.Record.Host, globex.Record.Value)

	res := verifyJoinDomain(t, jane, fixtures.GlobexPublicID, globex.ID)

	assert.IsType(t, &siteapi.SiteJoinDomainsVerifyConflict{}, res)
	acmeRows := listJoinDomains(t, john, fixtures.AcmePublicID)
	held, _ := lo.Find(acmeRows, func(d siteapi.SiteJoinDomainResource) bool { return d.ID == acme.ID })
	assert.True(t, held.Verified, "the holder keeps the domain")
	globexRows := listJoinDomains(t, jane, fixtures.GlobexPublicID)
	claim, _ := lo.Find(globexRows, func(d siteapi.SiteJoinDomainResource) bool { return d.ID == globex.ID })
	assert.False(t, claim.Verified, "the claimant stays pending")
}

func TestJoinDomainsDomainMovesWhenTheHoldersRecordIsGone(t *testing.T) {
	env, zone := joinDomainsEnv(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	acme := mustAddJoinDomain(t, john, fixtures.AcmePublicID, "changed-hands.test")
	zone.PublishTXT(t, acme.Record.Host, acme.Record.Value)
	require.IsType(t, &siteapi.SiteJoinDomainResource{}, verifyJoinDomain(t, john, fixtures.AcmePublicID, acme.ID))
	globex := mustAddJoinDomain(t, jane, fixtures.GlobexPublicID, "changed-hands.test")
	zone.WithdrawTXT(acme.Record.Host)
	zone.PublishTXT(t, globex.Record.Host, globex.Record.Value)

	res := verifyJoinDomain(t, jane, fixtures.GlobexPublicID, globex.ID)

	moved, ok := res.(*siteapi.SiteJoinDomainResource)
	require.Truef(t, ok, "got %T", res)
	assert.True(t, moved.Verified)
	acmeRows := listJoinDomains(t, john, fixtures.AcmePublicID)
	former, _ := lo.Find(acmeRows, func(d siteapi.SiteJoinDomainResource) bool { return d.ID == acme.ID })
	assert.False(t, former.Verified, "the previous holder keeps a pending claim")
	assert.False(t, former.VerifiedAt.Set && !former.VerifiedAt.Null)

	handedOver := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, "join_domain.update")
	require.NotEmpty(t, handedOver)
	assert.Equal(t, siteapi.SiteAuditActorKindSystem, handedOver[0].Actor.Kind, "the move is recorded under the system actor")
	gained := entriesNamed(t, env, fixtures.OwnerJaneEmail, fixtures.GlobexPublicID, "join_domain.update")
	require.NotEmpty(t, gained)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, gained[0].Actor.Kind)

	msgs := env.SystemMail.Messages()
	require.Len(t, msgs, 1, "the previous Owners are told, and only they")
	assert.Equal(t, fixtures.OwnerJohnEmail, msgs[0].To)
	assert.Contains(t, msgs[0].Subject, "changed-hands.test")
}

func TestJoinDomainsAMailFailureDoesNotUndoTheMove(t *testing.T) {
	env, zone := joinDomainsEnv(t)
	john := env.SiteActor(t, fixtures.OwnerJohnEmail)
	jane := env.SiteActor(t, fixtures.OwnerJaneEmail)
	acme := mustAddJoinDomain(t, john, fixtures.AcmePublicID, "mail-down.test")
	zone.PublishTXT(t, acme.Record.Host, acme.Record.Value)
	require.IsType(t, &siteapi.SiteJoinDomainResource{}, verifyJoinDomain(t, john, fixtures.AcmePublicID, acme.ID))
	globex := mustAddJoinDomain(t, jane, fixtures.GlobexPublicID, "mail-down.test")
	zone.WithdrawTXT(acme.Record.Host)
	zone.PublishTXT(t, globex.Record.Host, globex.Record.Value)
	env.SystemMail.SetErr(errors.New("smtp down"))

	res := verifyJoinDomain(t, jane, fixtures.GlobexPublicID, globex.ID)

	moved, ok := res.(*siteapi.SiteJoinDomainResource)
	require.Truef(t, ok, "got %T", res)
	assert.True(t, moved.Verified)
}

func TestJoinDomainsRemoveDeletesOnlyTheClaim(t *testing.T) {
	env := testhelper.Setup(t)
	ctx := context.Background()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ws := siteapi.WorkspaceId(fixtures.AcmePublicID)
	claim := mustAddJoinDomain(t, owner, ws, "remove-me.test")
	members := env.DB.Scoped(fixtures.AcmeID).Membership().Query().CountX(ctx)

	res, err := owner.SiteJoinDomainsRemove(ctx, siteapi.SiteJoinDomainsRemoveParams{WorkspaceId: ws, ID: claim.ID})
	require.NoError(t, err)

	assert.IsType(t, &siteapi.SiteJoinDomainsRemoveNoContent{}, res)
	left := listJoinDomains(t, owner, ws)
	_, still := lo.Find(left, func(d siteapi.SiteJoinDomainResource) bool { return d.ID == claim.ID })
	assert.False(t, still)
	after := env.DB.Scoped(fixtures.AcmeID).Membership().Query().CountX(ctx)
	assert.Equal(t, members, after, "removing a Join domain touches no Membership")
	gone, err := owner.SiteJoinDomainsRemove(ctx, siteapi.SiteJoinDomainsRemoveParams{WorkspaceId: ws, ID: claim.ID})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteJoinDomainsRemoveNotFound{}, gone)
	foreign, err := owner.SiteJoinDomainsRemove(ctx, siteapi.SiteJoinDomainsRemoveParams{WorkspaceId: ws, ID: siteapi.EntityId(fixtures.JoinDomainGlobexID)})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteJoinDomainsRemoveNotFound{}, foreign, "another Workspace's claim cannot be removed")
}

func TestJoinDomainsChangesAreAuditedWithoutTheToken(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	claim := mustAddJoinDomain(t, owner, fixtures.AcmePublicID, "audited.test")

	created := entriesNamed(t, env, fixtures.OwnerJohnEmail, fixtures.AcmePublicID, "join_domain.create")

	require.NotEmpty(t, created)
	assert.Equal(t, "audited.test", created[0].Target.Name.Value)
	diff, err := json.Marshal(created[0].Diff.Value)
	require.NoError(t, err)
	assert.NotContains(t, string(diff), claim.Record.Value, "the token stays out of the log")
}
