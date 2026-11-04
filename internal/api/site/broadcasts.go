package site

import (
	"context"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/broadcast"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/outbound"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/ratelimit"
)

// withStats maps a Broadcast and attaches its delivery figures, which are derived from its
// recipients and their Outbound messages rather than stored on it.
func withStats(ctx context.Context, s *ent.Scoped, b *ent.Broadcast) (siteapi.SiteBroadcastResource, error) {
	figures, err := broadcasts.StatsFor(ctx, s, b)
	if err != nil {
		return siteapi.SiteBroadcastResource{}, err
	}
	res := mapper.BroadcastToResource(b)
	res.Stats = mapper.BroadcastStatsToResource(figures[b.ID].Figures())
	return res, nil
}

// broadcastResource maps a Broadcast and, while it is sending, attaches its progress
// and ETA (ADR 0023), derived on the server from its recipients. Only the single-row
// responses (get, send) use it: the list leaves progress out.
func (h *Handlers) broadcastResource(ctx context.Context, s *ent.Scoped, b *ent.Broadcast) (siteapi.SiteBroadcastResource, error) {
	d, err := h.broadcasts.Detail(ctx, s, b, time.Now())
	if err != nil {
		return siteapi.SiteBroadcastResource{}, err
	}
	res := mapper.BroadcastToResource(b)
	res.Stats = mapper.BroadcastStatsToResource(d.Stats.Figures())
	if d.Progress != nil {
		res.Progress = siteapi.NewOptNilSiteBroadcastProgress(mapper.BroadcastProgressToResource(*d.Progress))
	}
	return res, nil
}

func (h *Handlers) SiteBroadcastsList(ctx context.Context, params siteapi.SiteBroadcastsListParams) (siteapi.SiteBroadcastsListRes, error) {
	ws := workspaceScope(ctx)

	order := inputs.BroadcastsOrder(params.Order.Or(siteapi.SiteBroadcastsOrderNewest))
	page, err := h.broadcasts.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), inputs.BroadcastsListFilter(params), order)
	if err != nil {
		return nil, err
	}

	// The figures of the whole page are two grouped queries. No progress here: it costs a
	// query per sending row and the list does not show it.
	figures, err := broadcasts.StatsFor(ctx, ws, page.Items...)
	if err != nil {
		return nil, err
	}
	return mapper.BroadcastsListPage(pagination.Map(page, func(b *ent.Broadcast) siteapi.SiteBroadcastResource {
		res := mapper.BroadcastToResource(b)
		res.Stats = mapper.BroadcastStatsToResource(figures[b.ID].Figures())
		return res
	})), nil
}

func (h *Handlers) SiteBroadcastsCreate(ctx context.Context, req *siteapi.CreateBroadcastInput, _ siteapi.SiteBroadcastsCreateParams) (siteapi.SiteBroadcastsCreateRes, error) {
	ws := workspaceScope(ctx)

	in, err := inputs.BroadcastCreate(req)
	if err != nil {
		return nil, err
	}
	b, err := h.broadcasts.Create(ctx, ws, in)
	if err != nil {
		return nil, err
	}
	res, err := withStats(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	if res.Warnings, err = h.broadcastWarnings(ctx, ws, b, req.Content.OneOf); err != nil {
		return nil, err
	}
	return &res, nil
}

// broadcastWarnings is the lint of a saved Broadcast's content on its Channel (ADR 0040), over the
// body as written when the save wrote one: on email the subject and the body.
func (h *Handlers) broadcastWarnings(ctx context.Context, s *ent.Scoped, b *ent.Broadcast, written siteapi.BroadcastContentInputSum) ([]siteapi.LintFinding, error) {
	if b.Channel == broadcast.ChannelEmail {
		body := written.EmailBroadcastContentInput.Body
		return h.lintWarnings(ctx, s, lo.FromPtr(b.Subject), body, lo.FromPtr(b.Body))
	}
	return nil, nil
}

func (h *Handlers) SiteBroadcastsGet(ctx context.Context, params siteapi.SiteBroadcastsGetParams) (siteapi.SiteBroadcastsGetRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	b, err := h.broadcasts.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := h.broadcastResource(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *Handlers) SiteBroadcastsUpdate(ctx context.Context, req *siteapi.UpdateBroadcastInput, params siteapi.SiteBroadcastsUpdateParams) (siteapi.SiteBroadcastsUpdateRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)

	in, err := inputs.BroadcastUpdate(req)
	if err != nil {
		return nil, err
	}
	b, err := h.broadcasts.Update(ctx, ws, id, in)
	if err != nil {
		return nil, err
	}
	res, err := withStats(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	if res.Warnings, err = h.broadcastWarnings(ctx, ws, b, req.Content.Value.OneOf); err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteBroadcastsArchive archives a draft, sent or failed broadcast (ADR 0100): it leaves the list
// but keeps its report, tracking and figures.
func (h *Handlers) SiteBroadcastsArchive(ctx context.Context, params siteapi.SiteBroadcastsArchiveParams) (siteapi.SiteBroadcastsArchiveRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	b, err := h.broadcasts.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := withStats(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteBroadcastsUnarchive brings an archived broadcast back to the list.
func (h *Handlers) SiteBroadcastsUnarchive(ctx context.Context, params siteapi.SiteBroadcastsUnarchiveParams) (siteapi.SiteBroadcastsUnarchiveRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	b, err := h.broadcasts.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := withStats(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteBroadcastsSend sends a draft or scheduled broadcast immediately. The state
// machine lives in the broadcasts module; this is the HTTP adapter.
func (h *Handlers) SiteBroadcastsSend(ctx context.Context, params siteapi.SiteBroadcastsSendParams) (siteapi.SiteBroadcastsSendRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)

	b, err := h.broadcasts.Send(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := h.broadcastResource(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteBroadcastsSchedule schedules a draft broadcast to send at a future time.
func (h *Handlers) SiteBroadcastsSchedule(ctx context.Context, req *siteapi.ScheduleBroadcastInput, params siteapi.SiteBroadcastsScheduleParams) (siteapi.SiteBroadcastsScheduleRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)

	b, err := h.broadcasts.Schedule(ctx, ws, id, time.Time(req.ScheduledAt))
	if err != nil {
		return nil, err
	}
	res, err := withStats(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteBroadcastsTestSend renders the broadcast with sample merge data and sends
// it to a single address — no recipient rows, no tracking (a token would point
// at a nonexistent recipient). Used to preview the rendered email or text.
func (h *Handlers) SiteBroadcastsTestSend(ctx context.Context, req *siteapi.TestSendBroadcastInput, params siteapi.SiteBroadcastsTestSendParams) (siteapi.SiteBroadcastsTestSendRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	b, err := h.broadcasts.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}

	// The asking User: with no verified Sending domain the preview may go only to them
	// (ADR 0041).
	a := auth.GetSiteAuth(ctx)
	if a == nil {
		return nil, auth.ErrUnauthorized
	}
	res, err := h.outbound.SendBroadcastTest(ctx, ws, b, inputs.TestAddress(req), &outbound.Asker{UserID: a.UserID, Email: a.Email})
	if err != nil {
		return nil, err
	}
	if res.Wait > 0 {
		// The System sender's cap: the client waits as for any rate limit.
		return nil, ratelimit.FromContext(ctx).Delay(ctx, ratelimit.PolicyTestSend, res.Limit, res.Wait, h.now())
	}
	out := mapper.TestSendResultToResource(res)
	return &out, nil
}
