package site

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// SiteCapturedMessagesList returns the Captured messages of the workspace's Test inbox, newest
// first. Every member may read.
func (h *Handlers) SiteCapturedMessagesList(ctx context.Context, params siteapi.SiteCapturedMessagesListParams) (siteapi.SiteCapturedMessagesListRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteCapturedMessagesListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.captured.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	res := make([]siteapi.SiteCapturedMessageResource, len(page.Items))
	for i, m := range page.Items {
		res[i] = mapper.CapturedMessageToResource(m)
	}
	return &siteapi.SiteCapturedMessagesListOK{
		Items:      res,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}
