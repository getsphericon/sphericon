package site

import (
	"context"
	"io"
	"net/http"
	"time"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/narrow"
)

// SiteAuditList shows the Workspace's Audit log, newest first. Owner and admin only;
// an instance without an Enterprise license answers 402 (ADR 0014, ADR 0022).
func (h *Handlers) SiteAuditList(ctx context.Context, params siteapi.SiteAuditListParams) (siteapi.SiteAuditListRes, error) {
	s := workspaceScope(ctx)
	if h.audit == nil || !h.audit.Licensed() {
		v := siteapi.SiteAuditListPaymentRequired(problem(http.StatusPaymentRequired, "the audit log needs an Enterprise license"))
		return &v, nil
	}

	filter := events.AuditFilter{
		From:       time.Time(params.From.Or(siteapi.Timestamp{})),
		To:         time.Time(params.To.Or(siteapi.Timestamp{})),
		ActorKind:  string(params.ActorKind.Or("")),
		ActorID:    params.ActorId.Or(""),
		Action:     params.Action.Or(""),
		TargetType: params.TargetType.Or(""),
		TargetID:   params.TargetId.Or(""),
		IP:         params.IP.Or(""),
		RequestID:  params.RequestId.Or(""),
	}
	cursor, err := auditapi.Page(params.Cursor.Or(""))
	limit := int(params.Limit.Value)
	if err != nil {
		return nil, err
	}

	rows, next, err := h.audit.Entries(ctx, s, filter, cursor, limit)
	if err != nil {
		return nil, err
	}
	out := &siteapi.SiteAuditEntryList{Items: mapper.AuditEntriesToResources(auditapi.ViewsOf(rows))}
	if next > 0 {
		out.NextCursor = siteapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}

// SiteAuditExport downloads the whole Audit log as CSV, newest first. Same access as
// the list: owner and admin only, 402 without an Enterprise license. It takes the
// same filter as SiteAuditList, so an export matches what the page shows.
func (h *Handlers) SiteAuditExport(ctx context.Context, params siteapi.SiteAuditExportParams) (siteapi.SiteAuditExportRes, error) {
	s := workspaceScope(ctx)
	if h.audit == nil || !h.audit.Licensed() {
		v := siteapi.SiteAuditExportPaymentRequired(problem(http.StatusPaymentRequired, "the audit log needs an Enterprise license"))
		return &v, nil
	}

	filter := events.AuditFilter{
		From:       time.Time(params.From.Or(siteapi.Timestamp{})),
		To:         time.Time(params.To.Or(siteapi.Timestamp{})),
		ActorKind:  string(params.ActorKind.Or("")),
		ActorID:    params.ActorId.Or(""),
		Action:     params.Action.Or(""),
		TargetType: params.TargetType.Or(""),
		TargetID:   params.TargetId.Or(""),
		IP:         params.IP.Or(""),
		RequestID:  params.RequestId.Or(""),
	}

	// A data export is itself audited (story 19): Export records it before any row leaves.
	write, err := h.audit.Export(ctx, s, filter)
	if err != nil {
		return nil, err
	}

	// Stream through a pipe so a long log never sits in memory. If the response ends
	// early the context is cancelled, which closes the pipe and stops the writer.
	pr, pw := io.Pipe()
	stop := context.AfterFunc(ctx, func() { _ = pw.CloseWithError(ctx.Err()) })
	go func() {
		defer stop()
		_ = pw.CloseWithError(write(pw))
	}()
	return &siteapi.SiteAuditExportOKHeaders{
		ContentDisposition: `attachment; filename="audit-log-` + string(params.WorkspaceId) + `.csv"`,
		Response:           siteapi.SiteAuditExportOK{Data: pr},
	}, nil
}

// SiteAuditGetRetention reads the Audit log retention window. Owner and admin only;
// 402 without the Enterprise retention license.
func (h *Handlers) SiteAuditGetRetention(ctx context.Context, _ siteapi.SiteAuditGetRetentionParams) (siteapi.SiteAuditGetRetentionRes, error) {
	s := workspaceScope(ctx)
	if h.audit == nil || !h.audit.RetentionLicensed() {
		v := siteapi.SiteAuditGetRetentionPaymentRequired(problem(http.StatusPaymentRequired, "audit retention needs an Enterprise license"))
		return &v, nil
	}
	ws, err := h.accounts.Workspace(ctx, s)
	if err != nil {
		return nil, err
	}
	return retentionResource(ws.RetentionDays), nil
}

// SiteAuditSetRetention sets or clears the Audit log retention window and records the
// change as a `workspace.update` Audit entry. Owner and admin only.
func (h *Handlers) SiteAuditSetRetention(ctx context.Context, req *siteapi.SiteAuditRetention, _ siteapi.SiteAuditSetRetentionParams) (siteapi.SiteAuditSetRetentionRes, error) {
	s := workspaceScope(ctx)
	if h.audit == nil || !h.audit.RetentionLicensed() {
		v := siteapi.SiteAuditSetRetentionPaymentRequired(problem(http.StatusPaymentRequired, "audit retention needs an Enterprise license"))
		return &v, nil
	}
	var days *int
	if !req.RetentionDays.Null {
		d := int(req.RetentionDays.Value)
		days = &d
	}
	if err := h.accounts.SetAuditRetention(ctx, s, h.actor(ctx), days); err != nil {
		return nil, err
	}
	return retentionResource(days), nil
}

func retentionResource(days *int) *siteapi.SiteAuditRetention {
	res := &siteapi.SiteAuditRetention{}
	if days == nil {
		res.RetentionDays.Null = true
	} else {
		res.RetentionDays.SetTo(siteapi.RetentionDays(narrow.Int32(*days)))
	}
	return res
}
