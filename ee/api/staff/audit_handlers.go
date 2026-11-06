package staff

import (
	"context"

	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
)

// StaffWorkspaceAuditList is a page of one Workspace's Audit log, newest first. An
// Staff member's entry shows as "Sphericon staff" with no id, as it does to the customer.
func (h *Handlers) StaffWorkspaceAuditList(ctx context.Context, params staffeeapi.StaffWorkspaceAuditListParams) (*staffeeapi.StaffAuditEntryList, error) {
	cursor, err := auditlog.Page(params.Cursor.Or(""))
	if err != nil {
		return nil, err
	}
	rows, next, err := h.module.AuditEntries(ctx, string(params.WorkspaceId), cursor, int(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	out := &staffeeapi.StaffAuditEntryList{Items: staffMapper.AuditEntriesToResources(auditlog.StaffViewsOf(rows))}
	if next > 0 {
		out.NextCursor = staffeeapi.NewOptNilString(auditlog.NextCursor(next))
	}
	return out, nil
}
