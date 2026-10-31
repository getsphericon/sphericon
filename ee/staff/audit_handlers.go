package staff

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/auditapi"
)

// StaffWorkspaceAuditList is a page of one Workspace's Audit log, newest first. An
// Staff member's entry shows as "Sphericon staff" with no id, as it does to the customer.
func (h *Handlers) StaffWorkspaceAuditList(ctx context.Context, params staffeeapi.StaffWorkspaceAuditListParams) (staffeeapi.StaffWorkspaceAuditListRes, error) {
	id := string(params.WorkspaceId)
	cursor, err := auditapi.Page(params.Cursor.Or(""))
	limit := int(params.Limit.Value)
	if err != nil {
		v := staffeeapi.StaffWorkspaceAuditListBadRequest{
			Status: staffeeapi.NewOptInt32(http.StatusBadRequest),
			Title:  staffeeapi.NewOptString(http.StatusText(http.StatusBadRequest)),
			Detail: staffeeapi.NewOptString(err.Error()),
		}
		return &v, nil
	}
	rows, next, err := h.module.AuditEntries(ctx, id, cursor, limit)
	if ent.IsNotFound(err) {
		v := staffeeapi.StaffWorkspaceAuditListNotFound(notFoundProblem())
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	out := &staffeeapi.StaffAuditEntryList{Items: staffMapper.AuditEntriesToResources(auditapi.StaffViewsOf(rows))}
	if next > 0 {
		out.NextCursor = staffeeapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}
