package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/segments"
)

func (h *Handlers) SegmentsList(ctx context.Context, params externalapi.SegmentsListParams) (externalapi.SegmentsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "segments:read") {
		res := externalapi.SegmentsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.segments.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &externalapi.SegmentsListOK{
		Items: lo.Map(page.Items, func(s *ent.Segment, _ int) externalapi.SegmentResource {
			return mapper.SegmentToResource(s)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SegmentsCreate(ctx context.Context, req *externalapi.CreateSegmentInput) (externalapi.SegmentsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "segments:write") {
		res := externalapi.SegmentsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	s, err := h.segments.Create(ctx, ws, segments.CreateInput{
		Name:       req.Name,
		Definition: &req.Definition,
	})
	if errors.Is(err, segments.ErrInvalidDefinition) {
		res := externalapi.SegmentsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsGet(ctx context.Context, params externalapi.SegmentsGetParams) (externalapi.SegmentsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "segments:read") {
		res := externalapi.SegmentsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	id, err := parseEntityID(params.ID)
	if err != nil {
		res := externalapi.SegmentsGetBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &res, nil
	}
	s, err := ws.Segment().Get(ctx, id)
	if ent.IsNotFound(err) {
		res := externalapi.SegmentsGetNotFound(problem(http.StatusNotFound, "segment not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsUpdate(ctx context.Context, req *externalapi.UpdateSegmentInput, params externalapi.SegmentsUpdateParams) (externalapi.SegmentsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "segments:write") {
		res := externalapi.SegmentsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	id, err := parseEntityID(params.ID)
	if err != nil {
		res := externalapi.SegmentsUpdateBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &res, nil
	}
	in := segments.UpdateInput{
		Name:       convert.StringPtr(req.Name),
		Definition: convert.StringPtr(req.Definition),
	}
	s, err := h.segments.Update(ctx, ws, id, in)
	if errors.Is(err, segments.ErrInvalidDefinition) {
		res := externalapi.SegmentsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	}
	if errors.Is(err, segments.ErrNotFound) {
		res := externalapi.SegmentsUpdateNotFound(problem(http.StatusNotFound, "segment not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsDelete(ctx context.Context, params externalapi.SegmentsDeleteParams) (externalapi.SegmentsDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "segments:write") {
		res := externalapi.SegmentsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	id, err := parseEntityID(params.ID)
	if err != nil {
		res := externalapi.SegmentsDeleteBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &res, nil
	}
	err = ws.Segment().DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		res := externalapi.SegmentsDeleteNotFound(problem(http.StatusNotFound, "segment not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.SegmentsDeleteNoContent{}, nil
}

func (h *Handlers) SegmentsPreview(ctx context.Context, req *externalapi.PreviewSegmentInput) (externalapi.SegmentsPreviewRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "segments:read") {
		res := externalapi.SegmentsPreviewUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	def := ""
	if v := convert.StringPtr(req.Definition); v != nil {
		def = *v
	}
	count, err := h.segments.Preview(ctx, ws, def)
	if errors.Is(err, segments.ErrInvalidDefinition) {
		res := externalapi.SegmentsPreviewUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.PreviewSegmentResult{Count: int32(count)}, nil
}
