package staff

import (
	"context"
	"encoding/json"

	staffapi "github.com/getsphericon/sphericon/gen/staff"
)

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
