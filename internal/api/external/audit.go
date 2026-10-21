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
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAuditRead) {
		res := externalapi.AuditEntriesListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	if h.audit == nil || !h.audit.Licensed() {
		res := externalapi.AuditEntriesListPaymentRequired(problem(http.StatusPaymentRequired, "the audit log needs an Enterprise license"))
		return &res, nil
	}

	cursor, limit, err := auditapi.Page(params.Cursor.Or(""), params.Limit.Or(0), params.Limit.IsSet())
	if err != nil {
		res := externalapi.AuditEntriesListBadRequest(problem(http.StatusBadRequest, "invalid cursor"))
		return &res, nil
	}
	filter := events.AuditFilter{
		From:       time.Time(params.From.Or(externalapi.Timestamp{})),
		To:         time.Time(params.To.Or(externalapi.Timestamp{})),
		ActorKind:  string(params.ActorKind.Or("")),
		ActorID:    params.ActorId.Or(""),
		Action:     params.Action.Or(""),
		TargetType: params.TargetType.Or(""),
		TargetID:   params.TargetId.Or(""),
		IP:         params.IP.Or(""),
		RequestID:  params.RequestId.Or(""),
	}

	rows, next, err := h.audit.Entries(ctx, auth.TokenScoped(ctx), filter, cursor, limit)
	if err != nil {
		return nil, err
	}
	out := &externalapi.AuditEntryList{Items: make([]externalapi.AuditEntryResource, len(rows))}
	for i, e := range rows {
		out.Items[i] = mapper.AuditEntryToResource(auditapi.ViewOf(e))
	}
	if next > 0 {
		out.NextCursor = externalapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}
