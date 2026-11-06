package site_test

import (
	"context"
	"encoding/json"
	"net/http"
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
	_, err := owner.SiteMembershipsUpdate(context.Background(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeMemberMembershipID)})
	require.NoError(t, err)
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
}

func auditList(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) (*siteapi.SiteAuditEntryList, error) {
	t.Helper()
	return c.SiteAuditList(context.Background(), siteapi.SiteAuditListParams{WorkspaceId: workspaceID})
}

func auditPage(t *testing.T, c *siteapi.Client, workspaceID siteapi.WorkspaceId) *siteapi.SiteAuditEntryList {
	t.Helper()
	page, err := auditList(t, c, workspaceID)
	require.NoError(t, err, "audit list")
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

	alice := env.SiteActor(t, fixtures.AdminAliceEmail)

	_, err := auditList(t, mary, fixtures.AcmePublicID)
	testhelper.SiteRefusal(t, err, http.StatusForbidden, "a member is refused")
	_, err = auditList(t, alice, fixtures.AcmePublicID)
	require.NoError(t, err, "an admin reads")

	oscar := env.SiteActor(t, fixtures.OutsiderOscarEmail)
	_, err = auditList(t, oscar, fixtures.AcmePublicID)
	testhelper.SiteRefusal(t, err, http.StatusNotFound, "a stranger finds no workspace")
}

func TestAuditIsTenantIsolated(t *testing.T) {
	env := testhelper.Setup(t)

	acme := auditPage(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)
	require.Len(t, acme.Items, 1, "Acme never sees Globex's entries")
	assert.EqualValues(t, fixtures.AcmeAuditEntryID, acme.Items[0].ID)

	globex := auditPage(t, env.SiteActor(t, fixtures.OwnerJaneEmail), fixtures.GlobexPublicID)
	require.Len(t, globex.Items, 5, "Globex sees only its own fixture entries, not Acme's")
	assert.EqualValues(t, fixtures.GlobexAuditEntryID, globex.Items[len(globex.Items)-1].ID, "oldest last")

	_, cross := auditList(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.GlobexPublicID)
	testhelper.SiteRefusal(t, cross, http.StatusNotFound, "John cannot address Globex's log")
}

func TestAuditRedeliveryDoesNotDuplicate(t *testing.T) {
	env := testhelper.Setup(t)
	promoteMary(t, env)
	env.RetryJobs(t, testhelper.EditionJobs...) // river redelivers the same envelope
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	page := auditPage(t, env.SiteActor(t, fixtures.OwnerJohnEmail), fixtures.AcmePublicID)
	assert.Len(t, page.Items, 2)
}

func TestAuditWithoutLicenseStoresNothingAndChangesNoBehaviour(t *testing.T) {
	env := testhelper.Setup(t, testhelper.WithoutLicense())
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	before, err := env.DB.AuditEntry.Query().Count(context.Background())
	require.NoError(t, err)

	// The mutation behaves as before: it succeeds and the role changes.
	updated, err := owner.SiteMembershipsUpdate(context.Background(),
		&siteapi.SiteUpdateMembershipInput{Role: siteapi.SiteMembershipRoleAdmin},
		siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeMemberMembershipID)})
	require.NoError(t, err)
	assert.Equal(t, siteapi.SiteMembershipRoleAdmin, updated.Role)

	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)
	stored, err := env.DB.AuditEntry.Query().Count(context.Background())
	require.NoError(t, err)
	assert.Equal(t, before, stored, "nothing was stored for the change")

	_, err = auditList(t, owner, fixtures.AcmePublicID)
	testhelper.SiteRefusal(t, err, http.StatusPaymentRequired, "the page is unavailable")
}

func TestAuditCursorPaginates(t *testing.T) {
	env := testhelper.Setup(t)
	owner := env.SiteActor(t, fixtures.OwnerJohnEmail)
	ctx := context.Background()
	for _, role := range []siteapi.SiteMembershipRole{siteapi.SiteMembershipRoleAdmin, siteapi.SiteMembershipRoleMember, siteapi.SiteMembershipRoleAdmin} {
		_, err := owner.SiteMembershipsUpdate(ctx, &siteapi.SiteUpdateMembershipInput{Role: role},
			siteapi.SiteMembershipsUpdateParams{WorkspaceId: fixtures.AcmePublicID, ID: idStr(fixtures.AcmeMemberMembershipID)})
		require.NoError(t, err)
	}
	env.PerformEnqueuedJobs(t, testhelper.EditionJobs...)

	first, err := owner.SiteAuditList(ctx, siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID, Limit: siteapi.NewOptInt32(2)})
	require.NoError(t, err)
	p1 := first
	require.Len(t, p1.Items, 2)
	require.True(t, p1.NextCursor.IsSet())

	second, err := owner.SiteAuditList(ctx, siteapi.SiteAuditListParams{
		WorkspaceId: fixtures.AcmePublicID, Limit: siteapi.NewOptInt32(2), Cursor: siteapi.NewOptString(p1.NextCursor.Value),
	})
	require.NoError(t, err)
	p2 := second
	require.Len(t, p2.Items, 2, "the remaining entries")
	assert.Empty(t, p2.NextCursor.Value, "last page")
	assert.NotEqual(t, p1.Items[1].ID, p2.Items[0].ID)

	_, err = owner.SiteAuditList(ctx, siteapi.SiteAuditListParams{WorkspaceId: fixtures.AcmePublicID, Cursor: siteapi.NewOptString("nope")})
	testhelper.SiteRefusal(t, err, http.StatusBadRequest)
}
