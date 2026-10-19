package site

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/samber/lo"
)

// broadcastResource maps a Broadcast and, while it is sending, attaches its progress
// and ETA (ADR 0023), derived on the server from its recipients. Only the single-row
// responses (get, send) use it: the list leaves progress out.
func broadcastResource(ctx context.Context, s *ent.Scoped, b *ent.Broadcast) (siteapi.SiteBroadcastResource, error) {
	res := mapper.BroadcastToResource(b)
	p, err := broadcasts.ProgressOf(ctx, s, b, time.Now())
	if err != nil || p == nil {
		return res, err
	}
	progress := siteapi.SiteBroadcastProgress{
		ProcessedCount: int32(p.Processed),
		RemainingCount: int32(p.Remaining),
	}
	if p.EstimatedCompletion != nil {
		progress.EstimatedCompletionAt = siteapi.NewOptNilTimestamp(siteapi.Timestamp(*p.EstimatedCompletion))
	}
	res.Progress = siteapi.NewOptNilSiteBroadcastProgress(progress)
	return res, nil
}

func (h *Handlers) SiteBroadcastsList(ctx context.Context, params siteapi.SiteBroadcastsListParams) (siteapi.SiteBroadcastsListRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBroadcastsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.broadcasts.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteBroadcastsListOK{
		// No progress here: it costs a query per sending row and the list does not show it.
		Items:      lo.Map(page.Items, func(b *ent.Broadcast, _ int) siteapi.SiteBroadcastResource { return mapper.BroadcastToResource(b) }),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteBroadcastsCreate(ctx context.Context, req *siteapi.SiteCreateBroadcastInput, params siteapi.SiteBroadcastsCreateParams) (siteapi.SiteBroadcastsCreateRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBroadcastsCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	segmentID := convert.Int64Ptr(req.SegmentId)
	integrationID := convert.Int64Ptr(req.IntegrationId)

	b, err := h.broadcasts.Create(ctx, ws, broadcasts.Fields{
		Name:          &req.Name,
		Subject:       convert.StringPtr(req.Subject),
		FromName:      convert.StringPtr(req.FromName),
		FromEmail:     convert.StringPtr(req.FromEmail),
		Body:          convert.StringPtr(req.Body),
		SegmentID:     segmentID,
		IntegrationID: integrationID,
	})
	if errors.Is(err, emailrender.ErrInvalidContent) {
		v := siteapi.SiteBroadcastsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) SiteBroadcastsGet(ctx context.Context, params siteapi.SiteBroadcastsGetParams) (siteapi.SiteBroadcastsGetRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBroadcastsGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	b, err := h.broadcasts.Get(ctx, ws, id)
	if errors.Is(err, broadcasts.ErrNotFound) {
		v := siteapi.SiteBroadcastsGetNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res, err := broadcastResource(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *Handlers) SiteBroadcastsUpdate(ctx context.Context, req *siteapi.SiteUpdateBroadcastInput, params siteapi.SiteBroadcastsUpdateParams) (siteapi.SiteBroadcastsUpdateRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBroadcastsUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)

	segmentID := convert.Int64Ptr(req.SegmentId)
	integrationID := convert.Int64Ptr(req.IntegrationId)

	// JSON Merge Patch: an explicit null clears the field, an absent key keeps it.
	b, err := h.broadcasts.Update(ctx, ws, id, broadcasts.Fields{
		Name:          convert.StringPtr(req.Name),
		Subject:       convert.StringPtr(req.Subject),
		FromName:      convert.StringPtr(req.FromName),
		FromEmail:     convert.StringPtr(req.FromEmail),
		Body:          convert.StringPtr(req.Body),
		SegmentID:     segmentID,
		IntegrationID: integrationID,

		ClearFromName:    req.FromName.IsNull(),
		ClearFromEmail:   req.FromEmail.IsNull(),
		ClearSegment:     req.SegmentId.IsNull(),
		ClearIntegration: req.IntegrationId.IsNull(),
	})
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		v := siteapi.SiteBroadcastsUpdateNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &v, nil
	case errors.Is(err, broadcasts.ErrNotDraft):
		v := siteapi.SiteBroadcastsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, "only draft broadcasts can be edited"))
		return &v, nil
	case errors.Is(err, emailrender.ErrInvalidContent):
		v := siteapi.SiteBroadcastsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) SiteBroadcastsDelete(ctx context.Context, params siteapi.SiteBroadcastsDeleteParams) (siteapi.SiteBroadcastsDeleteRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBroadcastsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.broadcasts.Delete(ctx, ws, id)
	if errors.Is(err, broadcasts.ErrNotFound) {
		v := siteapi.SiteBroadcastsDeleteNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteBroadcastsDeleteNoContent{}, nil
}

// SiteBroadcastsSend sends a draft or scheduled broadcast immediately. The state
// machine lives in the broadcasts module; this is the HTTP adapter.
func (h *Handlers) SiteBroadcastsSend(ctx context.Context, params siteapi.SiteBroadcastsSendParams) (siteapi.SiteBroadcastsSendRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBroadcastsSendNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)

	b, err := h.broadcasts.Send(ctx, ws, id)
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		v := siteapi.SiteBroadcastsSendNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &v, nil
	case errors.Is(err, broadcasts.ErrNotSendable):
		v := siteapi.SiteBroadcastsSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, i18n.T("errors.broadcast_already_sending", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res, err := broadcastResource(ctx, ws, b)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteBroadcastsSchedule schedules a draft broadcast to send at a future time.
func (h *Handlers) SiteBroadcastsSchedule(ctx context.Context, req *siteapi.SiteScheduleBroadcastInput, params siteapi.SiteBroadcastsScheduleParams) (siteapi.SiteBroadcastsScheduleRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBroadcastsScheduleNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)

	b, err := h.broadcasts.Schedule(ctx, ws, id, time.Time(req.ScheduledAt))
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		v := siteapi.SiteBroadcastsScheduleNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &v, nil
	case errors.Is(err, broadcasts.ErrNotSendable):
		v := siteapi.SiteBroadcastsScheduleUnprocessableEntity(problem(http.StatusUnprocessableEntity, i18n.T("errors.broadcast_already_sending", nil)))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

// SiteBroadcastsTestSend renders the broadcast with sample merge data and sends
// it to a single address — no recipient rows, no tracking (a token would point
// at a nonexistent recipient). Used to preview the rendered email.
func (h *Handlers) SiteBroadcastsTestSend(ctx context.Context, req *siteapi.SiteTestSendBroadcastInput, params siteapi.SiteBroadcastsTestSendParams) (siteapi.SiteBroadcastsTestSendRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteBroadcastsTestSendNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	b, err := h.broadcasts.Get(ctx, ws, id)
	if errors.Is(err, broadcasts.ErrNotFound) {
		v := siteapi.SiteBroadcastsTestSendNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	if detail := h.outbound.SendBroadcastTest(ctx, ws, b, string(req.Email)); detail != "" {
		v := siteapi.SiteBroadcastsTestSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, detail))
		return &v, nil
	}
	return &siteapi.SiteBroadcastsTestSendNoContent{}, nil
}
