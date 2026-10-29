package site

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/segments"
)

func (h *Handlers) SiteSegmentsList(ctx context.Context, params siteapi.SiteSegmentsListParams) (siteapi.SiteSegmentsListRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	page, err := h.segments.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteSegmentsListOK{
		Items: lo.Map(page.Items, func(s *ent.Segment, _ int) siteapi.SiteSegmentResource {
			return mapper.SegmentToResource(s)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteSegmentsCreate(ctx context.Context, req *siteapi.SiteCreateSegmentInput, params siteapi.SiteSegmentsCreateParams) (siteapi.SiteSegmentsCreateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	def, err := condition.Encode(&req.Condition)
	if err != nil {
		return nil, err
	}
	s, err := h.segments.Create(ctx, ws, segments.CreateInput{
		Name:      req.Name,
		Condition: &def,
	})
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SiteSegmentsGet(ctx context.Context, params siteapi.SiteSegmentsGetParams) (siteapi.SiteSegmentsGetRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	s, err := h.segments.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SiteSegmentsUpdate(ctx context.Context, req *siteapi.SiteUpdateSegmentInput, params siteapi.SiteSegmentsUpdateParams) (siteapi.SiteSegmentsUpdateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	in := segments.UpdateInput{Name: convert.StringPtr(req.Name)}
	in.Condition, err = condition.EncodeOptional(req.Condition)
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

// SiteSegmentsArchive archives a segment (ADR 0049): it leaves the list and new work, and a
// sent Broadcast's report still names it. A draft or scheduled Broadcast refuses it with 409.
func (h *Handlers) SiteSegmentsArchive(ctx context.Context, params siteapi.SiteSegmentsArchiveParams) (siteapi.SiteSegmentsArchiveRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.segments.Archive(ctx, ws, id)
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

// SiteSegmentsUnarchive restores an archived segment to the list.
func (h *Handlers) SiteSegmentsUnarchive(ctx context.Context, params siteapi.SiteSegmentsUnarchiveParams) (siteapi.SiteSegmentsUnarchiveRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.segments.Unarchive(ctx, ws, id)
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

// SiteSegmentsPreview reports how many *deliverable* (active) contacts match a
// rule definition. It applies the same active filter the broadcast send path
// uses, so the previewed number matches what a broadcast to this segment ships.
func (h *Handlers) SiteSegmentsPreview(ctx context.Context, req *siteapi.SitePreviewSegmentInput, params siteapi.SiteSegmentsPreviewParams) (siteapi.SiteSegmentsPreviewRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	def, err := condition.EncodeOptional(req.Condition)
	if err != nil {
		return nil, err
	}
	count, err := h.segments.Preview(ctx, ws, lo.FromPtr(def))
	if err != nil {
		return nil, err
	}
	return &siteapi.SitePreviewSegmentResult{Count: narrow.Int32(count)}, nil
}

// SiteSegmentContactsList lists the Contacts a Segment matches right now: its rule alone,
// like the Segment's count; Send-eligibility is subtracted only at send (ADR 0001).
func (h *Handlers) SiteSegmentContactsList(ctx context.Context, params siteapi.SiteSegmentContactsListParams) (siteapi.SiteSegmentContactsListRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	page, err := h.segments.Contacts(ctx, ws, int64(params.SegmentId), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSegmentContactsListOK{
		Items: lo.Map(page.Items, func(c *ent.Contact, _ int) siteapi.SiteContactResource {
			return mapper.ContactToResource(c)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}
