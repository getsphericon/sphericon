package staff

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/auditapi"
)

// StaffWorkspaceAuditList is a page of one Workspace's Audit log, newest first. An
// Staff member's entry shows as "Sphericon staff" with no id, as it does to the customer.
func (h *Handlers) StaffWorkspaceAuditList(ctx context.Context, params staffapi.StaffWorkspaceAuditListParams) (staffapi.StaffWorkspaceAuditListRes, error) {
	id := string(params.WorkspaceId)
	cursor, limit, err := auditapi.Page(params.Cursor.Or(""), params.Limit.Or(0), params.Limit.IsSet())
	if err != nil {
		v := staffapi.StaffWorkspaceAuditListBadRequest{
			Status: staffapi.NewOptInt32(http.StatusBadRequest),
			Title:  staffapi.NewOptString(http.StatusText(http.StatusBadRequest)),
			Detail: staffapi.NewOptString(err.Error()),
		}
		return &v, nil
	}
	rows, next, err := h.module.AuditEntries(ctx, id, cursor, limit)
	if ent.IsNotFound(err) {
		v := staffapi.StaffWorkspaceAuditListNotFound(notFoundProblem())
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	out := &staffapi.StaffAuditEntryList{Items: make([]staffapi.StaffAuditEntryResource, len(rows))}
	for i, e := range rows {
		out.Items[i] = staffMapper.AuditEntryToResource(auditapi.StaffViewOf(e))
	}
	if next > 0 {
		out.NextCursor = staffapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}
