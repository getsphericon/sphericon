package site

import (
	"context"
	"github.com/getsphericon/sphericon/internal/accounts"
	"io"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
)

// SiteAuditList shows the Workspace's Audit log, newest first. Owner and admin only;
// an instance without an Enterprise license answers 402 (ADR 0014, ADR 0022).
func (h *Handlers) SiteAuditList(ctx context.Context, params siteapi.SiteAuditListParams) (siteapi.SiteAuditListRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAuditListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageMembers(role) {
		v := siteapi.SiteAuditListForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}
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
	cursor, limit, err := auditapi.Page(params.Cursor.Or(""), params.Limit.Or(0), params.Limit.IsSet())
	if err != nil {
		v := siteapi.SiteAuditListBadRequest(problem(http.StatusBadRequest, "invalid cursor"))
		return &v, nil
	}

	rows, next, err := h.audit.Entries(ctx, s, filter, cursor, limit)
	if err != nil {
		return nil, err
	}
	out := &siteapi.SiteAuditEntryList{Items: make([]siteapi.SiteAuditEntryResource, len(rows))}
	for i, e := range rows {
		out.Items[i] = auditEntryResource(e)
	}
	if next > 0 {
		out.NextCursor = siteapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}

// SiteAuditExport downloads the whole Audit log as CSV, newest first. Same access as
// the list: owner and admin only, 402 without an Enterprise license. It takes the
// same filter as SiteAuditList, so an export matches what the page shows.
func (h *Handlers) SiteAuditExport(ctx context.Context, params siteapi.SiteAuditExportParams) (siteapi.SiteAuditExportRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAuditExportNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageMembers(role) {
		v := siteapi.SiteAuditExportForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}
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

	// A data export is itself audited (story 19), recorded before any row leaves.
	if err := h.recordAuditExport(ctx, s, filter); err != nil {
		return nil, err
	}

	// Stream through a pipe so a long log never sits in memory. If the response ends
	// early the context is cancelled, which closes the pipe and stops the writer.
	pr, pw := io.Pipe()
	stop := context.AfterFunc(ctx, func() { _ = pw.CloseWithError(ctx.Err()) })
	go func() {
		defer stop()
		_ = pw.CloseWithError(h.audit.ExportCSV(ctx, s, filter, pw))
	}()
	return &siteapi.SiteAuditExportOKHeaders{
		ContentDisposition: `attachment; filename="audit-log-` + params.Slug + `.csv"`,
		Response:           siteapi.SiteAuditExportOK{Data: pr},
	}, nil
}

func auditEntryResource(e *ent.AuditEntry) siteapi.SiteAuditEntryResource {
	v := auditapi.ViewOf(e)
	res := siteapi.SiteAuditEntryResource{
		ID:         siteapi.EntityId(v.ID),
		OccurredAt: siteapi.Timestamp(v.OccurredAt),
		Actor:      siteapi.SiteAuditActor{Kind: siteapi.SiteAuditActorKind(v.ActorKind), ID: nilableString(v.ActorID), Name: nilableString(v.ActorName)},
		Action:     v.Action,
		Target:     siteapi.SiteAuditTarget{Type: v.TargetType, ID: nilableString(v.TargetID), Name: nilableString(v.TargetName)},
		RequestId:  nilableString(v.RequestID),
		IP:         nilableString(v.IP),
		UserAgent:  nilableString(v.UserAgent),
	}
	if v.Diff != nil {
		res.Diff = siteapi.NewOptNilSiteAuditEntryResourceDiff(v.Diff)
	}
	return res
}

func nilableString(v *string) siteapi.OptNilString {
	if v == nil {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(*v)
}

// recordAuditExport records the export as an `audit_log.export` Audit entry whose diff
// names the filters it was narrowed by.
func (h *Handlers) recordAuditExport(ctx context.Context, s *ent.Scoped, f events.AuditFilter) error {
	filters := map[string]any{}
	if !f.From.IsZero() {
		filters["from"] = f.From.UTC().Format(time.RFC3339)
	}
	if !f.To.IsZero() {
		filters["to"] = f.To.UTC().Format(time.RFC3339)
	}
	for k, v := range map[string]string{
		"actor_kind": f.ActorKind, "actor_id": f.ActorID, "action": f.Action, "target_type": f.TargetType,
		"target_id": f.TargetID, "ip": f.IP, "request_id": f.RequestID,
	} {
		if v != "" {
			filters[k] = v
		}
	}
	entry := &events.AuditEntry{
		WorkspaceID: s.WorkspaceID(),
		Actor:       h.actor(ctx),
		Action:      events.ActionAuditLogExport,
		TargetType:  "audit_log",
	}
	if len(filters) > 0 {
		entry.Diff = map[string]any{"filter": filters}
	}
	return h.bus.WithinScopedTx(ctx, s, func(_ *ent.Scoped, pub events.Publisher) error {
		return events.RecordAudit(ctx, pub, entry)
	})
}

// SiteAuditGetRetention reads the Audit log retention window. Owner and admin only;
// 402 without the Enterprise retention license.
func (h *Handlers) SiteAuditGetRetention(ctx context.Context, params siteapi.SiteAuditGetRetentionParams) (siteapi.SiteAuditGetRetentionRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAuditGetRetentionNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageMembers(role) {
		v := siteapi.SiteAuditGetRetentionForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}
	if h.audit == nil || !h.audit.RetentionLicensed() {
		v := siteapi.SiteAuditGetRetentionPaymentRequired(problem(http.StatusPaymentRequired, "audit retention needs an Enterprise license"))
		return &v, nil
	}
	ws, err := s.Workspace(ctx)
	if err != nil {
		return nil, err
	}
	return retentionResource(ws.RetentionDays), nil
}

// SiteAuditSetRetention sets or clears the Audit log retention window and records the
// change as a `workspace.update` Audit entry. Owner and admin only.
func (h *Handlers) SiteAuditSetRetention(ctx context.Context, req *siteapi.SiteAuditRetention, params siteapi.SiteAuditSetRetentionParams) (siteapi.SiteAuditSetRetentionRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAuditSetRetentionNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageMembers(role) {
		v := siteapi.SiteAuditSetRetentionForbidden(problem(http.StatusForbidden, "insufficient role"))
		return &v, nil
	}
	if h.audit == nil || !h.audit.RetentionLicensed() {
		v := siteapi.SiteAuditSetRetentionPaymentRequired(problem(http.StatusPaymentRequired, "audit retention needs an Enterprise license"))
		return &v, nil
	}
	var days *int
	if !req.RetentionDays.Null {
		d := int(req.RetentionDays.Value)
		if d < minRetentionDays || d > maxRetentionDays {
			msg := i18n.T("errors.retention_days_range", nil)
			v := siteapi.SiteAuditSetRetentionUnprocessableEntity(problemWithErrors(
				http.StatusUnprocessableEntity, msg, map[string][]string{"retentionDays": {msg}}))
			return &v, nil
		}
		days = &d
	}
	if err := h.accounts.SetAuditRetention(ctx, s, h.actor(ctx), days); err != nil {
		return nil, err
	}
	return retentionResource(days), nil
}

const (
	minRetentionDays = 1
	maxRetentionDays = 3650
)

func retentionResource(days *int) *siteapi.SiteAuditRetention {
	res := &siteapi.SiteAuditRetention{}
	if days == nil {
		res.RetentionDays.Null = true
	} else {
		res.RetentionDays.SetTo(int32(*days))
	}
	return res
}
