package site

import (
	"context"
	"time"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/insights/auditlog"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// SiteAuditList shows the Workspace's Audit log, newest first. Owner and admin only;
// an instance without an Enterprise license answers 402 (ADR 0014, ADR 0022).
func (h *Handlers) SiteAuditList(ctx context.Context, params siteapi.SiteAuditListParams) (*siteapi.SiteAuditEntryList, error) {
	page, err := h.audit.Page(ctx, workspaceScope(ctx), auditlog.Query{
		Filter: events.AuditFilter{
			From:       time.Time(params.From.Or(siteapi.Timestamp{})),
			To:         time.Time(params.To.Or(siteapi.Timestamp{})),
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
	out := &siteapi.SiteAuditEntryList{Items: mapper.AuditEntriesToResources(page.Entries)}
	if page.Next != "" {
		out.NextCursor = siteapi.NewOptNilString(page.Next)
	}
	return out, nil
}

// SiteAuditFacets lists the actors, actions and target types that occur in the Workspace's
// Audit log, the choices of its filter. Same access as the list: owner and admin only, 402
// without an Enterprise license.
func (h *Handlers) SiteAuditFacets(ctx context.Context, _ siteapi.SiteAuditFacetsParams) (*siteapi.SiteAuditFacets, error) {
	facets, err := h.audit.Facets(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	out := mapper.AuditFacetsToResource(facets)
	return &out, nil
}

// SiteAuditExport downloads the whole Audit log as CSV, newest first. Same access as
// the list: owner and admin only, 402 without an Enterprise license. It takes the
// same filter as SiteAuditList, so an export matches what the page shows.
func (h *Handlers) SiteAuditExport(ctx context.Context, params siteapi.SiteAuditExportParams) (*siteapi.SiteAuditExportOKHeaders, error) {
	filter := events.AuditFilter{
		From:       time.Time(params.From.Or(siteapi.Timestamp{})),
		To:         time.Time(params.To.Or(siteapi.Timestamp{})),
		ActorKind:  string(params.ActorKind.Or("")),
		ActorID:    params.ActorId.Or(""),
		Action:     string(params.Action.Or("")),
		TargetType: params.TargetType.Or(""),
		TargetID:   params.TargetId.Or(""),
		IP:         params.IP.Or(""),
		RequestID:  params.RequestId.Or(""),
	}

	// A data export is itself audited (story 19): Export records it before any row leaves.
	rows, err := h.audit.Export(ctx, workspaceScope(ctx), filter)
	if err != nil {
		return nil, err
	}

	// The response reads a page of rows only once it has sent the last, so a long log never
	// sits in memory.
	return &siteapi.SiteAuditExportOKHeaders{
		ContentDisposition: `attachment; filename="audit-log-` + string(params.WorkspaceId) + `.csv"`,
		Response:           siteapi.SiteAuditExportOK{Data: seqreader.New(rows)},
	}, nil
}

// SiteAuditGetRetention reads the Audit log retention window. Owner and admin only;
// 402 without the Enterprise retention license.
func (h *Handlers) SiteAuditGetRetention(ctx context.Context, _ siteapi.SiteAuditGetRetentionParams) (*siteapi.SiteAuditRetention, error) {
	window, err := h.audit.Retention(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	return retentionResource(window), nil
}

// SiteAuditSetRetention sets or clears the Audit log retention window and records the
// change as a `workspace.update` Audit entry. Owner and admin only.
func (h *Handlers) SiteAuditSetRetention(ctx context.Context, req *siteapi.SiteAuditRetention, _ siteapi.SiteAuditSetRetentionParams) (*siteapi.SiteAuditRetention, error) {
	var days *int
	if !req.RetentionDays.Null {
		d := int(req.RetentionDays.Value)
		days = &d
	}
	window, err := h.audit.SetRetention(ctx, workspaceScope(ctx), int64(req.Revision), days)
	if err != nil {
		return nil, err
	}
	return retentionResource(window), nil
}

func retentionResource(window auditlog.RetentionWindow) *siteapi.SiteAuditRetention {
	res := &siteapi.SiteAuditRetention{Revision: siteapi.Revision(window.Revision)}
	if window.Days == nil {
		res.RetentionDays.Null = true
	} else {
		res.RetentionDays.SetTo(siteapi.RetentionDays(narrow.Int32(*window.Days)))
	}
	return res
}
