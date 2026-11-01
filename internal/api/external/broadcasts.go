package external

import (
	"context"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// Broadcast authoring over /api (ADR 0016): drafts, Segment, test send and report.
// Nothing here sends to the audience; scheduling and sending are separate,
// send-class operations. The state rules live in internal/broadcasts; these
// handlers check the scope, call it and map its errors.

func (h *Handlers) BroadcastsList(ctx context.Context, params externalapi.BroadcastsListParams) (externalapi.BroadcastsListRes, error) {
	ws := auth.TokenScoped(ctx)
	page, err := h.broadcasts.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false), nil, broadcasts.OrderNewest)
	if err != nil {
		return nil, err
	}
	return mapper.BroadcastsListPage(page), nil
}

func (h *Handlers) BroadcastsCreate(ctx context.Context, req *externalapi.CreateBroadcastInput) (externalapi.BroadcastsCreateRes, error) {
	ws := auth.TokenScoped(ctx)

	b, err := h.broadcasts.Create(ctx, ws, inputs.BroadcastCreate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsGet(ctx context.Context, params externalapi.BroadcastsGetParams) (externalapi.BroadcastsGetRes, error) {
	id := int64(params.ID)
	b, err := h.broadcasts.Get(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsUpdate(ctx context.Context, req *externalapi.UpdateBroadcastInput, params externalapi.BroadcastsUpdateParams) (externalapi.BroadcastsUpdateRes, error) {
	id := int64(params.ID)
	b, err := h.broadcasts.Update(ctx, auth.TokenScoped(ctx), id, inputs.BroadcastUpdate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsDelete(ctx context.Context, params externalapi.BroadcastsDeleteParams) (externalapi.BroadcastsDeleteRes, error) {
	id := int64(params.ID)
	err := h.broadcasts.DeleteDraft(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	return &externalapi.BroadcastsDeleteNoContent{}, nil
}

// BroadcastsArchive archives a sent or failed broadcast (ADR 0049).
func (h *Handlers) BroadcastsArchive(ctx context.Context, params externalapi.BroadcastsArchiveParams) (externalapi.BroadcastsArchiveRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	b, err := h.broadcasts.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

// BroadcastsUnarchive restores an archived broadcast.
func (h *Handlers) BroadcastsUnarchive(ctx context.Context, params externalapi.BroadcastsUnarchiveParams) (externalapi.BroadcastsUnarchiveRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	b, err := h.broadcasts.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsSetSegment(ctx context.Context, req *externalapi.SetBroadcastSegmentInput, params externalapi.BroadcastsSetSegmentParams) (externalapi.BroadcastsSetSegmentRes, error) {
	id := int64(params.ID)
	b, err := h.broadcasts.SetSegment(ctx, auth.TokenScoped(ctx), id, convert.Int64Ptr(req.SegmentId))
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

// BroadcastsSchedule and BroadcastsUnschedule are send-class (ADR 0016, "Send is a
// second lock"): they need broadcasts:send, which authoring scopes never imply. Over
// MCP they additionally need mcp:send (the x-mcp send flag).
func (h *Handlers) BroadcastsSchedule(ctx context.Context, req *externalapi.ScheduleBroadcastInput, params externalapi.BroadcastsScheduleParams) (externalapi.BroadcastsScheduleRes, error) {
	id := int64(params.ID)
	b, err := h.broadcasts.Schedule(ctx, auth.TokenScoped(ctx), id, time.Time(req.ScheduledAt))
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsUnschedule(ctx context.Context, params externalapi.BroadcastsUnscheduleParams) (externalapi.BroadcastsUnscheduleRes, error) {
	id := int64(params.ID)
	b, err := h.broadcasts.Unschedule(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

// BroadcastsTestSend renders the broadcast with sample merge data and sends it to
// one address: no recipient rows, no tracking, never the audience.
func (h *Handlers) BroadcastsTestSend(ctx context.Context, req *externalapi.TestSendBroadcastInput, params externalapi.BroadcastsTestSendParams) (externalapi.BroadcastsTestSendRes, error) {
	id := int64(params.ID)
	ws := auth.TokenScoped(ctx)
	b, err := h.broadcasts.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}

	// No Asker: /api never sends from the System sender, so its cap never defers the Test send.
	if _, err := h.outbound.SendBroadcastTest(ctx, ws, b, string(req.Email), nil); err != nil {
		return nil, err
	}
	return &externalapi.BroadcastsTestSendNoContent{}, nil
}

func (h *Handlers) BroadcastsReport(ctx context.Context, params externalapi.BroadcastsReportParams) (externalapi.BroadcastsReportRes, error) {
	id := int64(params.ID)
	r, err := h.broadcasts.Report(ctx, auth.TokenScoped(ctx), id)
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastReportToResource(r)
	return &res, nil
}
