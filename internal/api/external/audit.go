package external

import (
	"context"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// AuditEntriesList reads the Workspace's Audit log, newest first, for a token with
// the audit:read scope (a SIEM feed). An instance without an Enterprise license
// answers 402 (ADR 0014, ADR 0022).
func (h *Handlers) AuditEntriesList(ctx context.Context, params externalapi.AuditEntriesListParams) (*externalapi.AuditEntryList, error) {
	page, err := h.audit.Page(ctx, auth.TokenScoped(ctx), auditlog.Query{
		Filter: events.AuditFilter{
			From:       time.Time(params.From.Or(externalapi.Timestamp{})),
			To:         time.Time(params.To.Or(externalapi.Timestamp{})),
			ActorKind:  string(params.ActorKind.Or("")),
			ActorID:    params.ActorId.Or(""),
			Action:     string(params.Action.Or("")),
			TargetType: params.TargetType.Or(""),
			TargetID:   params.TargetId.Or(""),
			IP:         params.IP.Or(""),
			RequestID:  params.RequestId.Or(""),
		},
		Cursor: params.Cursor.Or(""),
		Limit:  int(params.Limit.Value),
	})
	if err != nil {
		return nil, err
	}
	out := &externalapi.AuditEntryList{Items: mapper.AuditEntriesToResources(page.Entries)}
	if page.Next != "" {
		out.NextCursor = externalapi.NewOptNilString(page.Next)
	}
	return out, nil
}
