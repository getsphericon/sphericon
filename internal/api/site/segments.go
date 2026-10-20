package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/segments"
)

func (h *Handlers) SiteSegmentsList(ctx context.Context, params siteapi.SiteSegmentsListParams) (siteapi.SiteSegmentsListRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSegmentsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.segments.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteSegmentsListOK{
		Items: lo.Map(page.Items, func(s *ent.Segment, _ int) siteapi.SiteSegmentResource {
			return mapper.SegmentToResource(s)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteSegmentsCreate(ctx context.Context, req *siteapi.SiteCreateSegmentInput, params siteapi.SiteSegmentsCreateParams) (siteapi.SiteSegmentsCreateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSegmentsCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	def, err := segments.Encode(&req.Definition)
	if err != nil {
		return nil, err
	}
	s, err := h.segments.Create(ctx, ws, segments.CreateInput{
		Name:       req.Name,
		Definition: &def,
	})
	if errors.Is(err, segments.ErrInvalidDefinition) {
		v := siteapi.SiteSegmentsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SiteSegmentsGet(ctx context.Context, params siteapi.SiteSegmentsGetParams) (siteapi.SiteSegmentsGetRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSegmentsGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	s, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSegmentsGetNotFound(problem(http.StatusNotFound, "segment not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SiteSegmentsUpdate(ctx context.Context, req *siteapi.SiteUpdateSegmentInput, params siteapi.SiteSegmentsUpdateParams) (siteapi.SiteSegmentsUpdateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSegmentsUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	in := segments.UpdateInput{Name: convert.StringPtr(req.Name)}
	if group, ok := req.Definition.Get(); ok {
		def, err := segments.Encode(&group)
		if err != nil {
			return nil, err
		}
		in.Definition = &def
	}
	s, err := h.segments.Update(ctx, ws, id, in)
	if errors.Is(err, segments.ErrInvalidDefinition) {
		v := siteapi.SiteSegmentsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	}
	if errors.Is(err, segments.ErrNotFound) {
		v := siteapi.SiteSegmentsUpdateNotFound(problem(http.StatusNotFound, "segment not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SiteSegmentsDelete(ctx context.Context, params siteapi.SiteSegmentsDeleteParams) (siteapi.SiteSegmentsDeleteRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSegmentsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	err = h.segments.Delete(ctx, ws, int64(params.ID))
	if refused, ok := inuse.As(err); ok {
		v := inUseProblem(refused)
		return &v, nil
	}
	if errors.Is(err, segments.ErrNotFound) {
		v := siteapi.SiteSegmentsDeleteNotFound(problem(http.StatusNotFound, "segment not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSegmentsDeleteNoContent{}, nil
}

// SiteSegmentsPreview reports how many *deliverable* (active) contacts match a
// rule definition. It applies the same active filter the broadcast send path
// uses, so the previewed number matches what a broadcast to this segment ships.
func (h *Handlers) SiteSegmentsPreview(ctx context.Context, req *siteapi.SitePreviewSegmentInput, params siteapi.SiteSegmentsPreviewParams) (siteapi.SiteSegmentsPreviewRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSegmentsPreviewNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	def := ""
	if group, ok := req.Definition.Get(); ok {
		if def, err = segments.Encode(&group); err != nil {
			return nil, err
		}
	}
	count, err := h.segments.Preview(ctx, ws, def)
	if errors.Is(err, segments.ErrInvalidDefinition) {
		v := siteapi.SiteSegmentsPreviewUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SitePreviewSegmentResult{Count: int32(count)}, nil
}
