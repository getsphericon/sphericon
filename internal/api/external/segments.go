package external

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SegmentsList(ctx context.Context, params externalapi.SegmentsListParams) (externalapi.SegmentsListRes, error) {
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
	ws := auth.TokenScoped(ctx)
	in, err := inputs.SegmentCreate(req)
	if err != nil {
		return nil, err
	}
	s, err := h.segments.Create(ctx, ws, in)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsGet(ctx context.Context, params externalapi.SegmentsGetParams) (externalapi.SegmentsGetRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	s, err := h.segments.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsUpdate(ctx context.Context, req *externalapi.UpdateSegmentInput, params externalapi.SegmentsUpdateParams) (externalapi.SegmentsUpdateRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	in, err := inputs.SegmentUpdate(req)
	if err != nil {
		return nil, err
	}
	s, err := h.segments.Update(ctx, ws, id, in)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

// SegmentsArchive archives a segment (ADR 0049); a draft Broadcast refuses it with 409.
func (h *Handlers) SegmentsArchive(ctx context.Context, params externalapi.SegmentsArchiveParams) (externalapi.SegmentsArchiveRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	s, err := h.segments.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

// SegmentsUnarchive restores an archived segment to the list.
func (h *Handlers) SegmentsUnarchive(ctx context.Context, params externalapi.SegmentsUnarchiveParams) (externalapi.SegmentsUnarchiveRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	s, err := h.segments.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SegmentsPreview(ctx context.Context, req *externalapi.PreviewSegmentInput) (externalapi.SegmentsPreviewRes, error) {
	ws := auth.TokenScoped(ctx)
	def := ""
	if group, ok := req.Condition.Get(); ok {
		var err error
		if def, err = condition.Encode(&group); err != nil {
			return nil, err
		}
	}
	count, err := h.segments.Preview(ctx, ws, def)
	if err != nil {
		return nil, err
	}
	return &externalapi.PreviewSegmentResult{Count: narrow.Int32(count)}, nil
}
