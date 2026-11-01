package site_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// promoteMary has Acme's owner raise the fixture member to admin (the audited seam),
// then delivers the published entry to the EE subscriber.
func promoteMary(t *testing.T, env *testhelper.TestEnv) {
	t.Helper()
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	res, err := owner.SiteMembershipsUpdate(context.Background(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeMemberMembershipID)})
	require.NoError(t, err)
	require.IsType(t, &siteapi.SiteMembershipResource{}, res)
	env.DeliverToEE(t)
}

func auditList(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) siteapi.SiteAuditListRes {
	t.Helper()
	res, err := c.SiteAuditList(context.Background(), siteapi.SiteAuditListParams{WorkspaceId: workspaceID})
	require.NoError(t, err)
	return res
}

func auditPage(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) *siteapi.SiteAuditEntryList {
	t.Helper()
	page, ok := auditList(t, c, workspaceID).(*siteapi.SiteAuditEntryList)
	require.Truef(t, ok, "audit list answered %T", auditList(t, c, workspaceID))
	return page
}

func TestAuditRoleChangeIsRecordedAndListed(t *testing.T) {
	env := testhelper.Setup(t)
	promoteMary(t, env)

	page := auditPage(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)
	require.Len(t, page.Items, 2, "the new entry and the fixture entry")
	e := page.Items[0]
	assert.Equal(t, siteapi.AuditActionMembershipUpdate, e.Action)
	assert.Equal(t, siteapi.SiteAuditActorKindUser, e.Actor.Kind)
	assert.Equal(t, strconv.Itoa(fixtures.OwnerJohnID), e.Actor.ID.Value)
	assert.Equal(t, "John", e.Actor.Name.Value)
	assert.Equal(t, "membership", e.Target.Type)
	assert.Equal(t, strconv.Itoa(fixtures.AcmeMemberMembershipID), e.Target.ID.Value)
	assert.Equal(t, "Mary", e.Target.Name.Value, "the target's name is snapshotted")
	assert.WithinDuration(t, time.Now(), time.Time(e.OccurredAt), time.Minute, "the time is recorded")

	diff, err := json.Marshal(e.Diff.Value)
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":{"from":"member","to":"admin"}}`, string(diff))
	assert.False(t, page.NextCursor.IsSet() && page.NextCursor.Value != "", "one entry fits one page")
}

func TestAuditIsNotAnEventAndHasNoSideEffectsOnProjection(t *testing.T) {
	env := testhelper.Setup(t)
	before, err := env.DB.Event.Query().Count(context.Background())
	require.NoError(t, err)
	promoteMary(t, env)

	after, err := env.DB.Event.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, after, "an Audit entry never becomes an Event")
}

func TestAuditReadIsOwnerAndAdminOnly(t *testing.T) {
	env := testhelper.Setup(t)
	mary := env.SiteActor(t, fixtures.MemberMaryEmail)

	assert.IsType(t, &siteapi.SiteAuditListForbidden{}, auditList(t, mary, fixtures.AcmePublicID), "a member is refused")

	promoteMary(t, env)
	assert.IsType(t, &siteapi.SiteAuditEntryList{}, auditList(t, mary, fixtures.AcmePublicID), "an admin reads")

	oscar := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	assert.IsType(t, &siteapi.SiteAuditListNotFound{}, auditList(t, oscar, fixtures.AcmePublicID), "a stranger finds no workspace")
}

func TestAuditIsTenantIsolated(t *testing.T) {
	env := testhelper.Setup(t)
	promoteMary(t, env)

	acme := auditPage(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)
	for _, e := range acme.Items {
		assert.NotEqual(t, int64(fixtures.GlobexAuditEntryID), int64(e.ID), "Acme never sees Globex's entry")
	}
	require.Len(t, acme.Items, 2)

	globex := auditPage(t, env.SiteActor(t, fixtures.OwnerJaneEmail), fixtures.GlobexPublicID)
	require.Len(t, globex.Items, 5, "Globex sees only its own fixture entries, not Acme's role change")
	assert.EqualValues(t, fixtures.GlobexAuditEntryID, globex.Items[len(globex.Items)-1].ID, "oldest last")

	cross := auditList(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.GlobexPublicID)
	assert.IsType(t, &siteapi.SiteAuditListNotFound{}, cross, "John cannot address Globex's log")
}

func TestAuditRedeliveryDoesNotDuplicate(t *testing.T) {
	env := testhelper.Setup(t)
	promoteMary(t, env)
	env.DeliverToEE(t) // the bus redelivers the same envelope

	page := auditPage(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)
	assert.Len(t, page.Items, 2)
}

func TestAuditWithoutLicenseStoresNothingAndChangesNoBehaviour(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before, err := env.DB.AuditEntry.Query().Count(context.Background())
	require.NoError(t, err)

	// The mutation behaves as before: it succeeds and the role changes.
	res, err := owner.SiteMembershipsUpdate(context.Background(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeMemberMembershipID)})
	require.NoError(t, err)
	updated, ok := res.(*siteapi.SiteMembershipResource)
	require.Truef(t, ok, "got %T", res)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, updated.Role)

	env.DeliverToEE(t)
	stored, err := env.DB.AuditEntry.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, stored, "nothing was stored for the change")

	assert.IsType(t, &siteapi.SiteAuditListPaymentRequired{}, auditList(t, owner, fixtures.AcmePublicID), "the page is unavailable")
}

func TestAuditCursorPaginates(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	for _, role := range []siteapi.SiteMembershipRole{siteapi.SiteMembershipRoleAdmin, siteapi.SiteMembershipRoleMember, siteapi.SiteMembershipRoleAdmin} {
		res, err := owner.SiteMembershipsUpdate(ctx, &siteapi.SiteUpdateMembershipInput{Role: role},
			siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeMemberMembershipID)})
		require.NoError(t, err)
		require.IsType(t, &siteapi.SiteMembershipResource{}, res)
	}
	env.DeliverToEE(t)

	first, err := owner.SiteAuditList(ctx, siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID, Limit: siteapi.NewOptInt32(2)})
	require.NoError(t, err)
	p1 := first.(*siteapi.SiteAuditEntryList)
	require.Len(t, p1.Items, 2)
	require.True(t, p1.NextCursor.IsSet())

	second, err := owner.SiteAuditList(ctx, siteapi.SiteAuditListParams{
		WorkspaceId: fixtures.AcmePublicID, Limit: siteapi.NewOptInt32(2), Cursor: siteapi.NewOptString(p1.NextCursor.Value),
	})
	require.NoError(t, err)
	p2 := second.(*siteapi.SiteAuditEntryList)
	require.Len(t, p2.Items, 2, "the remaining entries")
	assert.Empty(t, p2.NextCursor.Value, "last page")
	assert.NotEqual(t, p1.Items[1].ID, p2.Items[0].ID)

	bad, err := owner.SiteAuditList(ctx, siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID, Cursor: siteapi.NewOptString("nope")})
	require.NoError(t, err)
	assert.IsType(t, &siteapi.SiteAuditListBadRequest{}, bad)
}
