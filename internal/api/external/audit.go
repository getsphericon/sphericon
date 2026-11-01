package external

import (
	"context"
	"net/http"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/events"
)

// AuditEntriesList reads the Workspace's Audit log, newest first, for a token with
// the audit:read scope (a SIEM feed). An instance without an Enterprise license
// answers 402 (ADR 0014, ADR 0022).
func (h *Handlers) AuditEntriesList(ctx context.Context, params externalapi.AuditEntriesListParams) (externalapi.AuditEntriesListRes, error) {
	if h.audit == nil || !h.audit.Licensed() {
		res := externalapi.AuditEntriesListPaymentRequired(problem(http.StatusPaymentRequired, "the audit log needs an Enterprise license"))
		return &res, nil
	}

	cursor, err := auditapi.Page(params.Cursor.Or(""))
	limit := int(params.Limit.Value)
	if err != nil {
		return nil, err
	}
	filter := events.AuditFilter{
		From:       time.Time(params.From.Or(externalapi.Timestamp{})),
		To:         time.Time(params.To.Or(externalapi.Timestamp{})),
		ActorKind:  string(params.ActorKind.Or("")),
		ActorID:    params.ActorId.Or(""),
		Action:     string(params.Action.Or("")),
		TargetType: params.TargetType.Or(""),
		TargetID:   params.TargetId.Or(""),
		IP:         params.IP.Or(""),
		RequestID:  params.RequestId.Or(""),
	}

	rows, next, err := h.audit.Entries(ctx, auth.TokenScoped(ctx), filter, cursor, limit)
	if err != nil {
		return nil, err
	}
	out := &externalapi.AuditEntryList{Items: mapper.AuditEntriesToResources(auditapi.ViewsOf(rows))}
	if next > 0 {
		out.NextCursor = externalapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}
