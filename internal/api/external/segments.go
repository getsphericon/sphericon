package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) SegmentsList(ctx context.Context, params externalapi.SegmentsListParams) (*externalapi.SegmentsListOK, error) {
	page, err := h.segments.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return mapper.SegmentsListPage(page)
}

func (h *Handlers) SegmentsCreate(ctx context.Context, req *externalapi.CreateSegmentInput) (*externalapi.SegmentResource, error) {
	ws := auth.TokenScoped(ctx)
	in, err := inputs.SegmentCreate(req)
	if err != nil {
		return nil, err
	}
	s, err := h.segments.Create(ctx, ws, in)
	if err != nil {
		return nil, err
	}
	res, err := mapper.SegmentToResource(s)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *Handlers) SegmentsGet(ctx context.Context, params externalapi.SegmentsGetParams) (*externalapi.SegmentResource, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	s, err := h.segments.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := mapper.SegmentToResource(s)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (h *Handlers) SegmentsUpdate(ctx context.Context, req *externalapi.UpdateSegmentInput, params externalapi.SegmentsUpdateParams) (*externalapi.SegmentResource, error) {
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
	res, err := mapper.SegmentToResource(s)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SegmentsArchive archives a segment (ADR 0049); a draft Broadcast refuses it with 409.
func (h *Handlers) SegmentsArchive(ctx context.Context, params externalapi.SegmentsArchiveParams) (*externalapi.SegmentResource, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	s, err := h.segments.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := mapper.SegmentToResource(s)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SegmentsUnarchive restores an archived segment to the list.
func (h *Handlers) SegmentsUnarchive(ctx context.Context, params externalapi.SegmentsUnarchiveParams) (*externalapi.SegmentResource, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	s, err := h.segments.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res, err := mapper.SegmentToResource(s)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SegmentsPreview counts the contacts matching an unsaved rule definition, and, for a Broadcast's
// Channel, the recipients a Broadcast there would take now (ADR 0149).
func (h *Handlers) SegmentsPreview(ctx context.Context, req *externalapi.PreviewSegmentInput) (*externalapi.PreviewSegmentResult, error) {
	in, err := inputs.SegmentPreview(req)
	if err != nil {
		return nil, err
	}
	preview, err := h.segments.Preview(ctx, auth.TokenScoped(ctx), in)
	if err != nil {
		return nil, err
	}
	res := &externalapi.PreviewSegmentResult{Count: narrow.Int32(preview.Count)}
	if preview.Recipients != nil {
		res.Recipients = externalapi.NewOptInt32(narrow.Int32(*preview.Recipients))
	}
	return res, nil
}
