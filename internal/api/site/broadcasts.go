package site

import (
	"context"
	"time"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/broadcasts"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// broadcastResource maps a Broadcast's view and attaches what the converter cannot see: its
// delivery figures, its progress while it is sending (ADR 0023) and the lint warnings of a save
// (ADR 0040).
func broadcastResource(v broadcasts.View) siteapi.SiteBroadcastResource {
	res := mapper.BroadcastToResource(v.Broadcast)
	res.Stats = mapper.BroadcastStatsToResource(v.Stats.Figures())
	if v.Progress != nil {
		res.Progress = siteapi.NewOptNilSiteBroadcastProgress(mapper.BroadcastProgressToResource(*v.Progress))
	}
	res.Warnings = mapper.LintFindingsToResource(v.Warnings)
	return res
}

func (h *Handlers) SiteBroadcastsList(ctx context.Context, params siteapi.SiteBroadcastsListParams) (*siteapi.SiteBroadcastsListOK, error) {
	ws := workspaceScope(ctx)

	order := inputs.BroadcastsOrder(params.Order.Or(siteapi.SiteBroadcastsOrderNewest))
	page, err := h.broadcastViews.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), inputs.BroadcastsListFilter(params), order)
	if err != nil {
		return nil, err
	}
	return mapper.BroadcastsListPage(pagination.Map(page, broadcastResource)), nil
}

func (h *Handlers) SiteBroadcastsCreate(ctx context.Context, req *siteapi.CreateBroadcastInput, _ siteapi.SiteBroadcastsCreateParams) (*siteapi.SiteBroadcastResource, error) {
	ws := workspaceScope(ctx)

	in, err := inputs.BroadcastCreate(req)
	if err != nil {
		return nil, err
	}
	v, err := h.broadcastViews.Create(ctx, ws, in)
	if err != nil {
		return nil, err
	}
	res := broadcastResource(v)
	return &res, nil
}

func (h *Handlers) SiteBroadcastsGet(ctx context.Context, params siteapi.SiteBroadcastsGetParams) (*siteapi.SiteBroadcastResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	v, err := h.broadcastViews.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := broadcastResource(v)
	return &res, nil
}

func (h *Handlers) SiteBroadcastsUpdate(ctx context.Context, req *siteapi.UpdateBroadcastInput, params siteapi.SiteBroadcastsUpdateParams) (*siteapi.SiteBroadcastResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)

	in, err := inputs.BroadcastUpdate(req)
	if err != nil {
		return nil, err
	}
	v, err := h.broadcastViews.Update(ctx, ws, id, in)
	if err != nil {
		return nil, err
	}
	res := broadcastResource(v)
	return &res, nil
}

// SiteBroadcastsArchive archives a draft, sent or failed broadcast (ADR 0100): it leaves the list
// but keeps its report, tracking and figures.
func (h *Handlers) SiteBroadcastsArchive(ctx context.Context, params siteapi.SiteBroadcastsArchiveParams) (*siteapi.SiteBroadcastResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	v, err := h.broadcastViews.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := broadcastResource(v)
	return &res, nil
}

// SiteBroadcastsUnarchive brings an archived broadcast back to the list.
func (h *Handlers) SiteBroadcastsUnarchive(ctx context.Context, params siteapi.SiteBroadcastsUnarchiveParams) (*siteapi.SiteBroadcastResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	v, err := h.broadcastViews.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := broadcastResource(v)
	return &res, nil
}

// SiteBroadcastsSend sends a draft or scheduled broadcast immediately. The state
// machine lives in the broadcasts module; this is the HTTP adapter.
func (h *Handlers) SiteBroadcastsSend(ctx context.Context, req *siteapi.RevisionInput, params siteapi.SiteBroadcastsSendParams) (*siteapi.SiteBroadcastResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)

	v, err := h.broadcastViews.Send(ctx, ws, id, int64(req.Revision))
	if err != nil {
		return nil, err
	}
	res := broadcastResource(v)
	return &res, nil
}

// SiteBroadcastsSchedule schedules a draft broadcast to send at a future time.
func (h *Handlers) SiteBroadcastsSchedule(ctx context.Context, req *siteapi.ScheduleBroadcastInput, params siteapi.SiteBroadcastsScheduleParams) (*siteapi.SiteBroadcastResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)

	v, err := h.broadcastViews.Schedule(ctx, ws, id, int64(req.Revision), time.Time(req.ScheduledAt))
	if err != nil {
		return nil, err
	}
	res := broadcastResource(v)
	return &res, nil
}

// SiteBroadcastsTestSend renders the broadcast with sample merge data and sends
// it to a single address — no recipient rows, no tracking (a token would point
// at a nonexistent recipient). Used to preview the rendered email or text. The
// signed-in User asks: with no verified Sending domain the preview may go from the
// System sender to them only (ADR 0041).
func (h *Handlers) SiteBroadcastsTestSend(ctx context.Context, req *siteapi.TestSendBroadcastInput, params siteapi.SiteBroadcastsTestSendParams) (*siteapi.SiteTestSendBroadcastResult, error) {
	res, err := h.broadcasts.SendTest(ctx, workspaceScope(ctx), int64(params.ID), inputs.TestAddress(req))
	if err != nil {
		return nil, err
	}
	out := mapper.TestSendResultToResource(res)
	return &out, nil
}
