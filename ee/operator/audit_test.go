package operator_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	operatorapi "github.com/mokevnin/sphericon/gen/operator"
	"github.com/mokevnin/sphericon/internal/fixtures"
	"github.com/mokevnin/sphericon/internal/testhelper"
)

func (h *harness) auditEntries(t *testing.T, c *operatorapi.Client, workspaceID int, params operatorapi.OperatorWorkspaceAuditListParams) operatorapi.OperatorWorkspaceAuditListRes {
	t.Helper()
	params.WorkspaceId = operatorapi.EntityId(strconv.Itoa(workspaceID))
	res, err := c.OperatorWorkspaceAuditList(t.Context(), params)
	require.NoError(t, err)
	return res
}

func (h *harness) auditPage(t *testing.T, c *operatorapi.Client, workspaceID int, params operatorapi.OperatorWorkspaceAuditListParams) *operatorapi.OperatorAuditEntryList {
	t.Helper()
	page, ok := h.auditEntries(t, c, workspaceID, params).(*operatorapi.OperatorAuditEntryList)
	require.True(t, ok, "an audit page")
	return page
}

func TestTheWorkspaceAuditLogListsThatWorkspacesEntriesNewestFirst(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	page := h.auditPage(t, c, fixtures.GlobexID, operatorapi.OperatorWorkspaceAuditListParams{})

	ids := make([]string, 0, len(page.Items))
	for _, e := range page.Items {
		ids = append(ids, string(e.ID))
	}
	assert.Equal(t, []string{"6", "5", "4", "3", "1"}, ids, "only Globex's entries, newest first")
	assert.False(t, page.NextCursor.IsSet() && !page.NextCursor.IsNull())
	assert.Equal(t, "tag.create", page.Items[0].Action)
}

func TestTheWorkspaceAuditLogIsPaginatedByCursor(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	first := h.auditPage(t, c, fixtures.GlobexID, operatorapi.OperatorWorkspaceAuditListParams{Limit: operatorapi.NewOptInt32(2)})
	require.Len(t, first.Items, 2)
	cursor, ok := first.NextCursor.Get()
	require.True(t, ok)

	second := h.auditPage(t, c, fixtures.GlobexID, operatorapi.OperatorWorkspaceAuditListParams{
		Limit: operatorapi.NewOptInt32(2), Cursor: operatorapi.NewOptString(cursor),
	})
	require.Len(t, second.Items, 2)
	assert.Equal(t, operatorapi.EntityId("4"), second.Items[0].ID)
	assert.Equal(t, operatorapi.EntityId("3"), second.Items[1].ID)
}

func TestAnOperatorsEntryShowsAsSphericonStaffWithoutTheOperatorId(t *testing.T) {
	h := newHarness(t)
	c, cookie := h.operatorSession(t)

	page := h.auditPage(t, c, fixtures.InitechID, operatorapi.OperatorWorkspaceAuditListParams{})

	require.Len(t, page.Items, 2)
	staff := page.Items[1]
	assert.Equal(t, operatorapi.EntityId(strconv.Itoa(fixtures.InitechAuditOperatorSuspendID)), staff.ID)
	assert.Equal(t, operatorapi.OperatorAuditActorKindOperator, staff.Actor.Kind)
	assert.Equal(t, "sphericon staff", staff.Actor.Name.Value)
	assert.False(t, staff.Actor.ID.IsSet() && !staff.Actor.ID.IsNull(), "no Operator id")

	rec := get(t, h.env.Server, "/operator/workspaces/"+strconv.Itoa(fixtures.InitechID)+"/audit-entries", cookie)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "op-fixture")
	assert.NotContains(t, rec.Body.String(), "Jane Staff")
}

func TestTheWorkspaceAuditLogRefusesABadCursorAndAnUnknownWorkspace(t *testing.T) {
	h := newHarness(t)
	c, _ := h.operatorSession(t)

	bad := h.auditEntries(t, c, fixtures.GlobexID, operatorapi.OperatorWorkspaceAuditListParams{Cursor: operatorapi.NewOptString("nope")})
	assert.IsType(t, &operatorapi.OperatorWorkspaceAuditListBadRequest{}, bad)

	missing := h.auditEntries(t, c, 9999, operatorapi.OperatorWorkspaceAuditListParams{})
	assert.IsType(t, &operatorapi.OperatorWorkspaceAuditListNotFound{}, missing)
}

func TestTheWorkspaceAuditLogRequiresAnOperatorSession(t *testing.T) {
	h := newHarness(t)
	userToken := h.env.SiteToken(t, fixtures.OwnerJohnEmail, nil)
	path := "/operator/workspaces/" + strconv.Itoa(fixtures.AcmeID) + "/audit-entries"

	assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path).Code)
	assert.Equal(t, http.StatusUnauthorized, get(t, h.env.Server, path, &http.Cookie{Name: "JWT", Value: userToken}).Code)
}

func TestTheWorkspaceAuditLogAnswers404WithoutTheOperatorLicense(t *testing.T) {
	h := newHarness(t, testhelper.WithoutLicense())

	assert.Equal(t, http.StatusNotFound, get(t, h.env.Server, "/operator/workspaces/"+strconv.Itoa(fixtures.AcmeID)+"/audit-entries").Code)
}
