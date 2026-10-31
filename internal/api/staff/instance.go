package staff

import (
	"context"
	"encoding/json"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
	"github.com/getsphericon/sphericon/internal/auditapi"
	"github.com/getsphericon/sphericon/internal/staffmembers"
)

// StaffInstanceSetDefaultSendCap sets the Send cap of new Workspaces as the signed-in Staff member
// (ADR 0067); not found on an Instance that has no Send caps.
func (h *Handlers) StaffInstanceSetDefaultSendCap(ctx context.Context, req *staffapi.StaffDefaultSendCapInput) (staffapi.StaffInstanceSetDefaultSendCapRes, error) {
	perDay := int(req.PerDay)
	change, err := h.instance.SetDefaultSendCap(ctx, staffmembers.Current(ctx), &perDay)
	if err != nil {
		return nil, err
	}
	return new(mapper.DefaultSendCapChangeToResource(change)), nil
}

// StaffInstanceClearDefaultSendCap removes the Send cap of new Workspaces as the signed-in Staff
// member; not found on an Instance that has no Send caps.
func (h *Handlers) StaffInstanceClearDefaultSendCap(ctx context.Context) (staffapi.StaffInstanceClearDefaultSendCapRes, error) {
	change, err := h.instance.SetDefaultSendCap(ctx, staffmembers.Current(ctx), nil)
	if err != nil {
		return nil, err
	}
	return new(mapper.DefaultSendCapChangeToResource(change)), nil
}

// StaffInstanceListAuditEntries is a page of the Instance audit log (ADR 0070), newest first.
func (h *Handlers) StaffInstanceListAuditEntries(ctx context.Context, params staffapi.StaffInstanceListAuditEntriesParams) (staffapi.StaffInstanceListAuditEntriesRes, error) {
	cursor, err := auditapi.Page(params.Cursor.Or(""))
	if err != nil {
		return nil, err
	}
	rows, next, err := h.instance.AuditEntries(ctx, cursor, int(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	out := &staffapi.StaffInstanceAuditEntryList{Items: mapper.InstanceAuditEntriesToResources(rows)}
	if next > 0 {
		out.NextCursor = staffapi.NewOptNilString(auditapi.NextCursor(next))
	}
	return out, nil
}

// StaffInstanceGetState is the Instance's own settings and state (ADR 0032, ADR 0063).
func (h *Handlers) StaffInstanceGetState(ctx context.Context) (staffapi.StaffInstanceGetStateRes, error) {
	s, err := h.instance.State(ctx)
	if err != nil {
		return nil, err
	}
	return new(mapper.InstanceStateToResource(s)), nil
}

// StaffInstanceSetTelemetry stores the Staff member's Telemetry report choice; 409 while the
// environment decides.
func (h *Handlers) StaffInstanceSetTelemetry(ctx context.Context, req *staffapi.StaffInstanceTelemetryInput) (staffapi.StaffInstanceSetTelemetryRes, error) {
	s, err := h.instance.SetTelemetry(ctx, req.Enabled)
	if err != nil {
		return nil, err
	}
	return new(mapper.InstanceStateToResource(s)), nil
}

// StaffInstanceAcknowledgeNotice records that a Staff member saw the one-time notice, which the
// first report waits for.
func (h *Handlers) StaffInstanceAcknowledgeNotice(ctx context.Context) (staffapi.StaffInstanceAcknowledgeNoticeRes, error) {
	s, err := h.instance.AcknowledgeNotice(ctx)
	if err != nil {
		return nil, err
	}
	return new(mapper.InstanceStateToResource(s)), nil
}

// StaffInstancePreviewTelemetryReport is the Telemetry report exactly as the Instance would send
// it now, whether or not sending is on: the body of the same builder the daily sender uses.
func (h *Handlers) StaffInstancePreviewTelemetryReport(ctx context.Context) (staffapi.StaffInstancePreviewTelemetryReportRes, error) {
	report, err := h.instance.Report(ctx)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	var fields staffapi.StaffInstancePreviewTelemetryReportOK
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	return &fields, nil
}
