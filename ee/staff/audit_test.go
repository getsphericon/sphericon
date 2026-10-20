package staff_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

func (h *harness) auditEntries(t *testing.T, c *staffapi.Client, workspaceID string, params staffapi.StaffWorkspaceAuditListParams) staffapi.StaffWorkspaceAuditListRes {
	t.Helper()
	params.WorkspaceId = staffapi.WorkspaceId(workspaceID)
	res, err := c.StaffWorkspaceAuditList(t.Context(), params)
	require.NoError(t, err)
	return res
}

func (h *harness) auditPage(t *testing.T, c *staffapi.Client, workspaceID string, params staffapi.StaffWorkspaceAuditListParams) *staffapi.StaffAuditEntryList {
	t.Helper()
	page, ok := h.auditEntries(t, c, workspaceID, params).(*staffapi.StaffAuditEntryList)
	require.True(t, ok, "an audit page")
	return page
}

func TestTheWorkspaceAuditLogListsThatWorkspacesEntriesNewestFirst(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	page := h.auditPage(t, c, fixtures.GlobexPublicID, staffapi.StaffWorkspaceAuditListParams{})

	ids := make([]int64, 0, len(page.Items))
	for _, e := range page.Items {
		ids = append(ids, int64(e.ID))
	}
	assert.Equal(t, []int64{6, 5, 4, 3, 1}, ids, "only Globex's entries, newest first")
	assert.False(t, page.NextCursor.IsSet() && !page.NextCursor.IsNull())
	assert.Equal(t, "tag.create", page.Items[0].Action)
}

func TestTheWorkspaceAuditLogIsPaginatedByCursor(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	first := h.auditPage(t, c, fixtures.GlobexPublicID, staffapi.StaffWorkspaceAuditListParams{Limit: staffapi.NewOptInt32(2)})
	require.Len(t, first.Items, 2)
	cursor, ok := first.NextCursor.Get()
	require.True(t, ok)

	second := h.auditPage(t, c, fixtures.GlobexPublicID, staffapi.StaffWorkspaceAuditListParams{
		Limit: staffapi.NewOptInt32(2), Cursor: staffapi.NewOptString(cursor),
	})
	require.Len(t, second.Items, 2)
	assert.Equal(t, staffapi.EntityId(4), second.Items[0].ID)
	assert.Equal(t, staffapi.EntityId(3), second.Items[1].ID)
}

func TestAStaffMembersEntryShowsAsSphericonStaffWithoutTheStaffId(t *testing.T) {
	h := newHarness(t)
	c, cookie := h.staffSession(t)

	page := h.auditPage(t, c, fixtures.InitechPublicID, staffapi.StaffWorkspaceAuditListParams{})

	require.Len(t, page.Items, 2)
	staff := page.Items[1]
	assert.Equal(t, staffapi.EntityId(int64(fixtures.InitechAuditStaffSuspendID)), staff.ID)
	assert.Equal(t, staffapi.StaffAuditActorKindStaff, staff.Actor.Kind)
	assert.Equal(t, "Sphericon staff", staff.Actor.Name.Value)
	assert.False(t, staff.Actor.ID.IsSet() && !staff.Actor.ID.IsNull(), "no StaffMember id")

	rec := get(t, h.env.Server, "/staff/workspaces/"+fixtures.InitechPublicID+"/audit-entries", cookie)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "op-fixture")
	assert.NotContains(t, rec.Body.String(), "Jane Staff")
}

func TestTheWorkspaceAuditLogRefusesABadCursorAndAnUnknownWorkspace(t *testing.T) {
	h := newHarness(t)
	c, _ := h.staffSession(t)

	bad := h.auditEntries(t, c, fixtures.GlobexPublicID, staffapi.StaffWorkspaceAuditListParams{Cursor: staffapi.NewOptString("nope")})
	assert.IsType(t, &staffapi.StaffWorkspaceAuditListBadRequest{}, bad)

	missing := h.auditEntries(t, c, "NoSuchId", staffapi.StaffWorkspaceAuditListParams{})
	assert.IsType(t, &staffapi.StaffWorkspaceAuditListNotFound{}, missing)
}

func TestTheWorkspaceAuditLogRequiresAnStaffSession(t *testing.T) {
	h := newHarness(t)
	userToken := h.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	path := "/staff/workspaces/" + fixtures.AcmePublicID + "/audit-entries"

	assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path).Code)
	assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path, &http.Cookie{Name: "JWT", Value: userToken}).Code)
}

func TestTheWorkspaceAuditLogAnswers404WithoutTheStaffLicense(t *testing.T) {
	h := newHarness(t, testhelper.WithoutLicense())

	assert.Equal(t, http.StatusNotFound, get(t, h.env.Server, "/staff/workspaces/"+fixtures.AcmePublicID+"/audit-entries").Code)
}
