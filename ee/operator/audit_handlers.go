package operator

import (
	"context"
	"net/http"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	operatorapi "github.com/getsphericon/sphericon/gen/operator"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/events"
)

// OperatorWorkspaceAuditList is a page of one Workspace's Audit log, newest first. An
// Operator's entry shows as "sphericon staff" with no id, as it does to the customer.
func (h *Handlers) OperatorWorkspaceAuditList(ctx context.Context, params operatorapi.OperatorWorkspaceAuditListParams) (operatorapi.OperatorWorkspaceAuditListRes, error) {
	id, err := strconv.ParseInt(string(params.WorkspaceId), 10, 64)
	if err != nil {
		v := operatorapi.OperatorWorkspaceAuditListNotFound(notFoundProblem())
		return &v, nil
	}
	cursor, limit, err := auditapi.Page(params.Cursor.Or(""), params.Limit.Or(0), params.Limit.IsSet())
	if err != nil {
		v := operatorapi.OperatorWorkspaceAuditListBadRequest{
			Status: operatorapi.NewOptInt32(http.StatusBadRequest),
			Title:  operatorapi.NewOptString(http.StatusText(http.StatusBadRequest)),
			Detail: operatorapi.NewOptString(err.Error()),
		}
		return &v, nil
	}
	rows, next, err := h.module.AuditEntries(ctx, id, cursor, limit)
	if ent.IsNotFound(err) {
		v := operatorapi.OperatorWorkspaceAuditListNotFound(notFoundProblem())
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	out := &operatorapi.OperatorAuditEntryList{Items: make([]operatorapi.OperatorAuditEntryResource, len(rows))}
	for i, e := range rows {
		out.Items[i] = auditEntryResource(e)
	}
	if next > 0 {
		out.NextCursor = operatorapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}

func auditEntryResource(e *ent.AuditEntry) operatorapi.OperatorAuditEntryResource {
	v := auditapi.ViewOf(e)
	res := operatorapi.OperatorAuditEntryResource{
		ID:         operatorapi.EntityId(v.ID),
		OccurredAt: operatorapi.Timestamp(v.OccurredAt),
		Actor:      operatorapi.OperatorAuditActor{Kind: operatorapi.OperatorAuditActorKind(v.ActorKind), ID: nilableString(v.ActorID), Name: nilableString(v.ActorName)},
		Action:     v.Action,
		Billing:    v.Billing,
		Target:     operatorapi.OperatorAuditTarget{Type: v.TargetType, ID: nilableString(v.TargetID), Name: nilableString(v.TargetName)},
		RequestId:  nilableString(v.RequestID),
		IP:         nilableString(v.IP),
		UserAgent:  nilableString(v.UserAgent),
	}
	// The console is the platform's own view: a Service credential is named here, as a
	// customer's read of the same entry shows "sphericon staff".
	if e.ActorKind == events.ActorService {
		res.Actor = operatorapi.OperatorAuditActor{Kind: operatorapi.OperatorAuditActorKindService, ID: nilableString(e.ActorID), Name: nilableString(e.ActorName)}
	}
	if v.Diff != nil {
		res.Diff = operatorapi.NewOptNilOperatorAuditEntryResourceDiff(v.Diff)
	}
	return res
}

func nilableString(v *string) operatorapi.OptNilString {
	if v == nil {
		return operatorapi.OptNilString{}
	}
	return operatorapi.NewOptNilString(*v)
}
