package external

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/broadcasts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/emailrender"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// Broadcast authoring over /api (ADR 0016): drafts, audience, test send and report.
// Nothing here sends to the audience; scheduling and sending are separate,
// send-class operations. The state rules live in internal/broadcasts; these
// handlers check the scope, call it and map its errors.

const detailNotDraft = "only draft broadcasts can be changed"

func (h *Handlers) BroadcastsList(ctx context.Context, params externalapi.BroadcastsListParams) (externalapi.BroadcastsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsRead) {
		res := externalapi.BroadcastsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	ws := auth.TokenScoped(ctx)
	page, err := h.broadcasts.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return &externalapi.BroadcastsListOK{
		Items:      lo.Map(page.Items, func(b *ent.Broadcast, _ int) externalapi.BroadcastResource { return mapper.BroadcastToResource(b) }),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) BroadcastsCreate(ctx context.Context, req *externalapi.CreateBroadcastInput) (externalapi.BroadcastsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsWrite) {
		res := externalapi.BroadcastsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	ws := auth.TokenScoped(ctx)

	b, err := h.broadcasts.Create(ctx, ws, broadcasts.Fields{
		Name: &req.Name, Subject: convert.StringPtr(req.Subject), FromName: convert.StringPtr(req.FromName),
		FromEmail: convert.StringPtr(req.FromEmail), Body: convert.StringPtr(req.Body),
		Doc: convert.DocJSON(req.Doc.Value, req.Doc.Set),
	})
	if errors.Is(err, broadcasts.ErrBlankName) {
		res := externalapi.BroadcastsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, "name must not be empty"))
		return &res, nil
	}
	if errors.Is(err, broadcasts.ErrBodyAndDoc) || errors.Is(err, emailrender.ErrInvalidContent) {
		res := externalapi.BroadcastsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsGet(ctx context.Context, params externalapi.BroadcastsGetParams) (externalapi.BroadcastsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsRead) {
		res := externalapi.BroadcastsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	b, err := h.broadcasts.Get(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, broadcasts.ErrNotFound) {
		res := externalapi.BroadcastsGetNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsUpdate(ctx context.Context, req *externalapi.UpdateBroadcastInput, params externalapi.BroadcastsUpdateParams) (externalapi.BroadcastsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsWrite) {
		res := externalapi.BroadcastsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	b, err := h.broadcasts.Update(ctx, auth.TokenScoped(ctx), id,
		broadcasts.Fields{
			Name: convert.StringPtr(req.Name), Subject: convert.StringPtr(req.Subject), FromName: convert.StringPtr(req.FromName),
			FromEmail: convert.StringPtr(req.FromEmail), Body: convert.StringPtr(req.Body),
			Doc: convert.DocJSON(req.Doc.Value, req.Doc.Set),
		})
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		res := externalapi.BroadcastsUpdateNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	case errors.Is(err, broadcasts.ErrNotDraft):
		res := externalapi.BroadcastsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, detailNotDraft))
		return &res, nil
	case errors.Is(err, broadcasts.ErrBlankName):
		res := externalapi.BroadcastsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, "name must not be empty"))
		return &res, nil
	case errors.Is(err, broadcasts.ErrBodyAndDoc), errors.Is(err, emailrender.ErrInvalidContent):
		res := externalapi.BroadcastsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsDelete(ctx context.Context, params externalapi.BroadcastsDeleteParams) (externalapi.BroadcastsDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsWrite) {
		res := externalapi.BroadcastsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	err := h.broadcasts.DeleteDraft(ctx, auth.TokenScoped(ctx), id)
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		res := externalapi.BroadcastsDeleteNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	case errors.Is(err, broadcasts.ErrNotDraft):
		res := externalapi.BroadcastsDeleteUnprocessableEntity(problem(http.StatusUnprocessableEntity, detailNotDraft))
		return &res, nil
	case err != nil:
		return nil, err
	}
	return &externalapi.BroadcastsDeleteNoContent{}, nil
}

// BroadcastsArchive archives a sent or failed broadcast (ADR 0049).
func (h *Handlers) BroadcastsArchive(ctx context.Context, params externalapi.BroadcastsArchiveParams) (externalapi.BroadcastsArchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsWrite) {
		res := externalapi.BroadcastsArchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	err := h.broadcasts.Archive(ctx, ws, id)
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		res := externalapi.BroadcastsArchiveNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	case errors.Is(err, broadcasts.ErrNotArchivable):
		res := externalapi.BroadcastsArchiveUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	case err != nil:
		return nil, err
	}
	b, err := h.broadcasts.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

// BroadcastsUnarchive restores an archived broadcast.
func (h *Handlers) BroadcastsUnarchive(ctx context.Context, params externalapi.BroadcastsUnarchiveParams) (externalapi.BroadcastsUnarchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsWrite) {
		res := externalapi.BroadcastsUnarchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	err := h.broadcasts.Unarchive(ctx, ws, id)
	if errors.Is(err, broadcasts.ErrNotFound) {
		res := externalapi.BroadcastsUnarchiveNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	b, err := h.broadcasts.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsSetAudience(ctx context.Context, req *externalapi.SetBroadcastAudienceInput, params externalapi.BroadcastsSetAudienceParams) (externalapi.BroadcastsSetAudienceRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsWrite) {
		res := externalapi.BroadcastsSetAudienceUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	var segmentID *int64
	if !req.SegmentId.Null {
		sid := int64(req.SegmentId.Value)
		segmentID = &sid
	}
	b, err := h.broadcasts.SetAudience(ctx, auth.TokenScoped(ctx), id, segmentID)
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		res := externalapi.BroadcastsSetAudienceNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	case errors.Is(err, broadcasts.ErrSegmentNotFound):
		res := externalapi.BroadcastsSetAudienceUnprocessableEntity(problem(http.StatusUnprocessableEntity, "segment not found"))
		return &res, nil
	case errors.Is(err, broadcasts.ErrNotDraft):
		res := externalapi.BroadcastsSetAudienceUnprocessableEntity(problem(http.StatusUnprocessableEntity, detailNotDraft))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

// BroadcastsSchedule and BroadcastsUnschedule are send-class (ADR 0016, "Send is a
// second lock"): they need broadcasts:send, which authoring scopes never imply. Over
// MCP they additionally need mcp:send (the x-mcp send flag).
func (h *Handlers) BroadcastsSchedule(ctx context.Context, req *externalapi.ScheduleBroadcastInput, params externalapi.BroadcastsScheduleParams) (externalapi.BroadcastsScheduleRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsSend) {
		res := externalapi.BroadcastsScheduleUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	b, err := h.broadcasts.Schedule(ctx, auth.TokenScoped(ctx), id, time.Time(req.ScheduledAt))
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		res := externalapi.BroadcastsScheduleNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	case errors.Is(err, broadcasts.ErrNotSendable):
		res := externalapi.BroadcastsScheduleUnprocessableEntity(problem(http.StatusUnprocessableEntity, "only draft or scheduled broadcasts can be scheduled"))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

func (h *Handlers) BroadcastsUnschedule(ctx context.Context, params externalapi.BroadcastsUnscheduleParams) (externalapi.BroadcastsUnscheduleRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsSend) {
		res := externalapi.BroadcastsUnscheduleUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	b, err := h.broadcasts.Unschedule(ctx, auth.TokenScoped(ctx), id)
	switch {
	case errors.Is(err, broadcasts.ErrNotFound):
		res := externalapi.BroadcastsUnscheduleNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	case errors.Is(err, broadcasts.ErrNotScheduled):
		res := externalapi.BroadcastsUnscheduleUnprocessableEntity(problem(http.StatusUnprocessableEntity, "only scheduled broadcasts can be unscheduled"))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.BroadcastToResource(b)
	return &res, nil
}

// BroadcastsTestSend renders the broadcast with sample merge data and sends it to
// one address: no recipient rows, no tracking, never the audience.
func (h *Handlers) BroadcastsTestSend(ctx context.Context, req *externalapi.TestSendBroadcastInput, params externalapi.BroadcastsTestSendParams) (externalapi.BroadcastsTestSendRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsWrite) {
		res := externalapi.BroadcastsTestSendUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	ws := auth.TokenScoped(ctx)
	b, err := h.broadcasts.Get(ctx, ws, id)
	if errors.Is(err, broadcasts.ErrNotFound) {
		res := externalapi.BroadcastsTestSendNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}

	if detail := h.outbound.SendBroadcastTest(ctx, auth.TokenScoped(ctx), b, string(req.Email), nil).Detail; detail != "" {
		res := externalapi.BroadcastsTestSendUnprocessableEntity(problem(http.StatusUnprocessableEntity, detail))
		return &res, nil
	}
	return &externalapi.BroadcastsTestSendNoContent{}, nil
}

func (h *Handlers) BroadcastsReport(ctx context.Context, params externalapi.BroadcastsReportParams) (externalapi.BroadcastsReportRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeBroadcastsRead) {
		res := externalapi.BroadcastsReportUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	r, err := h.broadcasts.Report(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, broadcasts.ErrNotFound) {
		res := externalapi.BroadcastsReportNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.BroadcastReport{
		Status:            externalapi.BroadcastStatus(r.Status),
		HoldReason:        optNilString(r.HoldReason),
		RecipientsTotal:   narrow.Int32(r.Recipients),
		SentCount:         narrow.Int32(r.Sent),
		SkippedCount:      narrow.Int32(r.Skipped),
		FailedCount:       narrow.Int32(r.Failed),
		OpenedCount:       narrow.Int32(r.Opened),
		ClickedCount:      narrow.Int32(r.Clicked),
		UnsubscribedCount: narrow.Int32(r.Unsubscribed),
		DeliveredCount:    narrow.Int32(r.Delivered),
		BouncedCount:      narrow.Int32(r.Bounced),
		ComplainedCount:   narrow.Int32(r.Complained),
		OpenRate:          r.OpenRate,
		ClickRate:         r.ClickRate,
	}, nil
}

func optNilString(v *string) externalapi.OptNilString {
	if v == nil {
		return externalapi.OptNilString{}
	}
	return externalapi.NewOptNilString(*v)
}
