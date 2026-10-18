package site

import (
	"context"
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/instance"
)

// instanceOwner reports whether the caller is an Owner of the oldest Workspace, the only
// users who may read or change the Instance state (ADR 0032).
func (h *Handlers) instanceOwner(ctx context.Context) (bool, error) {
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return false, nil
	}
	return h.instance.IsOwner(ctx, a.UserID)
}

func instanceForbidden() siteapi.ProblemDetails {
	return problem(http.StatusForbidden, i18n.T("errors.instance_owner_only", nil))
}

func instanceState(s instance.State) *siteapi.SiteInstanceState {
	t := siteapi.SiteInstanceTelemetry{Enabled: s.Telemetry.Enabled, Locked: s.Telemetry.Locked}
	if s.Telemetry.Locked {
		t.LockedBy = siteapi.NewOptSiteInstanceTelemetryLock(siteapi.SiteInstanceTelemetryLock(s.Telemetry.LockedBy))
	}
	return &siteapi.SiteInstanceState{Telemetry: t, NoticeAcknowledged: s.NoticeAcknowledged}
}

// SiteInstanceGetState returns the Instance state to an Owner of the oldest Workspace.
func (h *Handlers) SiteInstanceGetState(ctx context.Context) (siteapi.SiteInstanceGetStateRes, error) {
	if ok, err := h.instanceOwner(ctx); err != nil {
		return nil, err
	} else if !ok {
		v := instanceForbidden()
		return &v, nil
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
	if ok, err := h.instanceOwner(ctx); err != nil {
		return nil, err
	} else if !ok {
		v := siteapi.SiteInstanceSetTelemetryForbidden(instanceForbidden())
		return &v, nil
	}
	s, err := h.instance.SetTelemetry(ctx, req.Enabled)
	if errors.Is(err, instance.ErrTelemetryLocked) {
		v := siteapi.SiteInstanceSetTelemetryConflict(problem(http.StatusConflict, i18n.T("errors.telemetry_locked", nil)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return instanceState(s), nil
}

// SiteInstanceAcknowledgeNotice records that an Owner saw the one-time notice.
func (h *Handlers) SiteInstanceAcknowledgeNotice(ctx context.Context) (siteapi.SiteInstanceAcknowledgeNoticeRes, error) {
	if ok, err := h.instanceOwner(ctx); err != nil {
		return nil, err
	} else if !ok {
		v := instanceForbidden()
		return &v, nil
	}
	s, err := h.instance.AcknowledgeNotice(ctx)
	if err != nil {
		return nil, err
	}
	return instanceState(s), nil
}
