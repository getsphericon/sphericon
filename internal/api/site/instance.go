package site

import (
	"context"
	"encoding/json"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/instance"
)

// authorizeInstance is nil when the caller holds instance:manage in the oldest Workspace, the
// only users who may read or change the Instance state (ADR 0032, ADR 0061), and otherwise the
// Permission's refusal, which the server answers 403 permission_required.
func (h *Handlers) authorizeInstance(ctx context.Context) error {
	// The security handler lets no anonymous caller through; one would be no User, refused.
	var userID int64
	if a := auth.GetSiteAuth(ctx); a != nil {
		userID = a.UserID
	}
	return h.instance.Authorize(ctx, userID)
}

func instanceState(s instance.State) *siteapi.SiteInstanceState {
	t := siteapi.SiteInstanceTelemetry{Enabled: s.Telemetry.Enabled, Locked: s.Telemetry.Locked}
	if s.Telemetry.Locked {
		t.LockedBy = siteapi.NewOptSiteInstanceTelemetryLock(siteapi.SiteInstanceTelemetryLock(s.Telemetry.LockedBy))
	}
	out := &siteapi.SiteInstanceState{Telemetry: t, NoticeAcknowledged: s.NoticeAcknowledged, TelemetrySchemaUrl: s.SchemaURL}
	if s.LatestRelease != "" {
		out.LatestRelease = siteapi.NewOptString(s.LatestRelease)
	}
	if !s.LicenseExpiredAt.IsZero() {
		out.LicenseExpiredAt = siteapi.NewOptTimestamp(siteapi.Timestamp(s.LicenseExpiredAt))
	}
	return out
}

// SiteInstanceGetState returns the Instance state to an Owner of the oldest Workspace.
func (h *Handlers) SiteInstanceGetState(ctx context.Context) (siteapi.SiteInstanceGetStateRes, error) {
	if err := h.authorizeInstance(ctx); err != nil {
		return nil, err
	}
	s, err := h.instance.State(ctx)
	if err != nil {
		return nil, err
	}
	return instanceState(s), nil
}

// SiteInstanceSetTelemetry stores the Owner's Telemetry report choice; 409 while the
// environment decides.
func (h *Handlers) SiteInstanceSetTelemetry(ctx context.Context, req *siteapi.SiteInstanceTelemetryInput) (siteapi.SiteInstanceSetTelemetryRes, error) {
	if err := h.authorizeInstance(ctx); err != nil {
		return nil, err
	}
	s, err := h.instance.SetTelemetry(ctx, req.Enabled)
	if err != nil {
		return nil, err
	}
	return instanceState(s), nil
}

// SiteInstanceAcknowledgeNotice records that an Owner saw the one-time notice.
func (h *Handlers) SiteInstanceAcknowledgeNotice(ctx context.Context) (siteapi.SiteInstanceAcknowledgeNoticeRes, error) {
	if err := h.authorizeInstance(ctx); err != nil {
		return nil, err
	}
	s, err := h.instance.AcknowledgeNotice(ctx)
	if err != nil {
		return nil, err
	}
	return instanceState(s), nil
}

// SiteInstancePreviewTelemetryReport returns the Telemetry report exactly as the Instance
// would send it now, whether or not sending is on. It is the body of the same builder the
// daily sender uses, so the Owner reads the real payload.
func (h *Handlers) SiteInstancePreviewTelemetryReport(ctx context.Context) (siteapi.SiteInstancePreviewTelemetryReportRes, error) {
	if err := h.authorizeInstance(ctx); err != nil {
		return nil, err
	}
	report, err := h.instance.Report(ctx)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	var fields siteapi.SiteInstancePreviewTelemetryReportOK
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	return &fields, nil
}
