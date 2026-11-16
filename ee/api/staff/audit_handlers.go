package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	staffeeapi "github.com/getsphericon/sphericon/gen/staffee"
)

// StaffWorkspaceAuditList is a page of one Workspace's Audit log, newest first. An
// Staff member's entry shows as "Sphericon staff" with no id, as it does to the customer.
func (h *Handlers) StaffWorkspaceAuditList(ctx context.Context, params staffeeapi.StaffWorkspaceAuditListParams) (*staffeeapi.StaffAuditEntryList, error) {
	page, err := h.module.AuditEntries(ctx, string(params.WorkspaceId), workspaceoversight.AuditEntriesInput{Cursor: params.Cursor.Or(""), Limit: int(params.Limit.Value)})
	if err != nil {
		return nil, err
	}
	out := &staffeeapi.StaffAuditEntryList{Items: staffMapper.AuditEntriesToResources(page.Entries)}
	if page.Next != "" {
		out.NextCursor = staffeeapi.NewOptNilString(page.Next)
	}
	return out, nil
}
