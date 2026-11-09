package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/narrow"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) SiteSegmentsList(ctx context.Context, params siteapi.SiteSegmentsListParams) (*siteapi.SiteSegmentsListOK, error) {
	ws := workspaceScope(ctx)

	page, err := h.segments.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}

	return mapper.SegmentsListPage(page), nil
}

func (h *Handlers) SiteSegmentsCreate(ctx context.Context, req *siteapi.SiteCreateSegmentInput, _ siteapi.SiteSegmentsCreateParams) (*siteapi.SiteSegmentResource, error) {
	ws := workspaceScope(ctx)

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

func (h *Handlers) SiteSegmentsGet(ctx context.Context, params siteapi.SiteSegmentsGetParams) (*siteapi.SiteSegmentResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	s, err := h.segments.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

func (h *Handlers) SiteSegmentsUpdate(ctx context.Context, req *siteapi.SiteUpdateSegmentInput, params siteapi.SiteSegmentsUpdateParams) (*siteapi.SiteSegmentResource, error) {
	ws := workspaceScope(ctx)

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

// SiteSegmentsArchive archives a segment (ADR 0049): it leaves the list and new work, and a
// sent Broadcast's report still names it. A draft Broadcast refuses it with 409.
func (h *Handlers) SiteSegmentsArchive(ctx context.Context, params siteapi.SiteSegmentsArchiveParams) (*siteapi.SiteSegmentResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	s, err := h.segments.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

// SiteSegmentsUnarchive restores an archived segment to the list.
func (h *Handlers) SiteSegmentsUnarchive(ctx context.Context, params siteapi.SiteSegmentsUnarchiveParams) (*siteapi.SiteSegmentResource, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	s, err := h.segments.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.SegmentToResource(s)
	return &res, nil
}

// SiteSegmentsPreview reports how many contacts match a rule definition, and, for a Broadcast
// form that names its Channel, how many of them the Broadcast there would take: the audience of
// its plan, not the Segment's whole match.
func (h *Handlers) SiteSegmentsPreview(ctx context.Context, req *siteapi.SitePreviewSegmentInput, _ siteapi.SiteSegmentsPreviewParams) (*siteapi.SitePreviewSegmentResult, error) {
	in, err := inputs.SegmentPreview(req)
	if err != nil {
		return nil, err
	}
	preview, err := h.segments.Preview(ctx, workspaceScope(ctx), in)
	if err != nil {
		return nil, err
	}
	res := &siteapi.SitePreviewSegmentResult{Count: narrow.Int32(preview.Count)}
	if preview.Recipients != nil {
		res.Recipients = siteapi.NewOptInt32(narrow.Int32(*preview.Recipients))
	}
	return res, nil
}

// SiteSegmentContactsList lists the Contacts a Segment matches right now: its rule alone,
// like the Segment's count; Send-eligibility is subtracted only at send (ADR 0001).
func (h *Handlers) SiteSegmentContactsList(ctx context.Context, params siteapi.SiteSegmentContactsListParams) (*siteapi.SiteSegmentContactsListOK, error) {
	ws := workspaceScope(ctx)

	page, err := h.segments.Contacts(ctx, ws, int64(params.SegmentId), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.SegmentContactsListPage(page), nil
}
