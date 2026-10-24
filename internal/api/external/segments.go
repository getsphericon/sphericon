package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/segments"
	"github.com/getsphericon/sphericon/internal/validation"
)

func (h *Handlers) SegmentsList(ctx context.Context, params externalapi.SegmentsListParams) (externalapi.SegmentsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSegmentsRead) {
		res := externalapi.SegmentsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.segments.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return &externalapi.SegmentsListOK{
		Items: lo.Map(page.Items, func(s *ent.Segment, _ int) externalapi.SegmentResource {
			return mapper.SegmentToResource(s)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SegmentsCreate(ctx context.Context, req *externalapi.CreateSegmentInput) (externalapi.SegmentsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSegmentsWrite) {
		res := externalapi.SegmentsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	def, err := condition.Encode(&req.Condition)
	if err != nil {
		return nil, err
	}
	s, err := h.segments.Create(ctx, ws, segments.CreateInput{
		Name:      req.Name,
		Condition: &def,
	})
	if errors.Is(err, segments.ErrInvalidCondition) {
		res := externalapi.SegmentsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsGet(ctx context.Context, params externalapi.SegmentsGetParams) (externalapi.SegmentsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSegmentsRead) {
		res := externalapi.SegmentsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	s, err := h.segments.Get(ctx, ws, id)
	if errors.Is(err, segments.ErrNotFound) {
		res := externalapi.SegmentsGetNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsUpdate(ctx context.Context, req *externalapi.UpdateSegmentInput, params externalapi.SegmentsUpdateParams) (externalapi.SegmentsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSegmentsWrite) {
		res := externalapi.SegmentsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	cond, err := condition.EncodeOptional(req.Condition)
	if err != nil {
		return nil, err
	}
	in := segments.UpdateInput{Name: convert.StringPtr(req.Name), Condition: cond}
	s, err := h.segments.Update(ctx, ws, id, in)
	if errors.Is(err, segments.ErrInvalidCondition) {
		res := externalapi.SegmentsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &res, nil
	}
	if errors.Is(err, segments.ErrNotFound) {
		res := externalapi.SegmentsUpdateNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

// SegmentsArchive archives a segment (ADR 0049); a draft or scheduled Broadcast refuses it with 409.
func (h *Handlers) SegmentsArchive(ctx context.Context, params externalapi.SegmentsArchiveParams) (externalapi.SegmentsArchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSegmentsWrite) {
		res := externalapi.SegmentsArchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	err := h.segments.Archive(ctx, ws, id)
	if refused, ok := inuse.As(err); ok {
		res := inUseProblem(refused)
		return &res, nil
	}
	if errors.Is(err, segments.ErrNotFound) {
		res := externalapi.SegmentsArchiveNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	s, err := h.segments.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

// SegmentsUnarchive restores an archived segment to the list.
func (h *Handlers) SegmentsUnarchive(ctx context.Context, params externalapi.SegmentsUnarchiveParams) (externalapi.SegmentsUnarchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSegmentsWrite) {
		res := externalapi.SegmentsUnarchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	err := h.segments.Unarchive(ctx, ws, id)
	if errors.Is(err, segments.ErrNotFound) {
		res := externalapi.SegmentsUnarchiveNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	s, err := h.segments.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsPreview(ctx context.Context, req *externalapi.PreviewSegmentInput) (externalapi.SegmentsPreviewRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeSegmentsRead) {
		res := externalapi.SegmentsPreviewUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	ws := auth.TokenScoped(ctx)
	def := ""
	if group, ok := req.Condition.Get(); ok {
		var err error
		if def, err = condition.Encode(&group); err != nil {
			return nil, err
		}
	}
	count, err := h.segments.Preview(ctx, ws, def)
	if errors.Is(err, segments.ErrInvalidCondition) {
		res := externalapi.SegmentsPreviewUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.PreviewSegmentResult{Count: narrow.Int32(count)}, nil
}
