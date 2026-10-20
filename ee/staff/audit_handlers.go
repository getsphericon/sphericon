package staff

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/events"
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
		out.Items[i] = auditEntryResource(e)
	}
	if next > 0 {
		out.NextCursor = staffapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}

func auditEntryResource(e *ent.AuditEntry) staffapi.StaffAuditEntryResource {
	v := auditapi.ViewOf(e)
	res := staffapi.StaffAuditEntryResource{
		ID:         staffapi.EntityId(v.ID),
		OccurredAt: staffapi.Timestamp(v.OccurredAt),
		Actor:      staffapi.StaffAuditActor{Kind: staffapi.StaffAuditActorKind(v.ActorKind), ID: nilableString(v.ActorID), Name: nilableString(v.ActorName)},
		Action:     v.Action,
		Billing:    v.Billing,
		Target:     staffapi.StaffAuditTarget{Type: v.TargetType, ID: nilableString(v.TargetID), Name: nilableString(v.TargetName)},
		RequestId:  nilableString(v.RequestID),
		IP:         nilableString(v.IP),
		UserAgent:  nilableString(v.UserAgent),
	}
	// The console is the platform's own view: a Service credential is named here, as a
	// customer's read of the same entry shows "Sphericon staff".
	if e.ActorKind == events.ActorService {
		res.Actor = staffapi.StaffAuditActor{Kind: staffapi.StaffAuditActorKindService, ID: nilableString(e.ActorID), Name: nilableString(e.ActorName)}
	}
	if v.Diff != nil {
		res.Diff = staffapi.NewOptNilStaffAuditEntryResourceDiff(v.Diff)
	}
	return res
}

func nilableString(v *string) staffapi.OptNilString {
	if v == nil {
		return staffapi.OptNilString{}
	}
	return staffapi.NewOptNilString(*v)
}
