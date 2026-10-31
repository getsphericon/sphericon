package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// SiteCapturedMessagesList returns the Captured messages of the workspace's Test inbox, newest
// first. Every member may read.
func (h *Handlers) SiteCapturedMessagesList(ctx context.Context, params siteapi.SiteCapturedMessagesListParams) (siteapi.SiteCapturedMessagesListRes, error) {
	s := workspaceScope(ctx)

	page, err := h.captured.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	out := mapper.CapturedMessagesListPage(page)
	return &out, nil
}

// SiteCapturedMessagesGet returns one Captured message in full. Every member may read.
func (h *Handlers) SiteCapturedMessagesGet(ctx context.Context, params siteapi.SiteCapturedMessagesGetParams) (siteapi.SiteCapturedMessagesGetRes, error) {
	s := workspaceScope(ctx)
	row, err := h.captured.Get(ctx, s, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.CapturedMessageToResource(row)
	return &res, nil
}

// SiteCapturedMessagesDelete deletes one Captured message. Every member may delete.
func (h *Handlers) SiteCapturedMessagesDelete(ctx context.Context, params siteapi.SiteCapturedMessagesDeleteParams) (siteapi.SiteCapturedMessagesDeleteRes, error) {
	s := workspaceScope(ctx)
	err := h.captured.Delete(ctx, s, int64(params.ID))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteCapturedMessagesDeleteNoContent{}, nil
}

// SiteCapturedMessagesDeleteAll deletes every Captured message of the workspace. Every member
// may delete.
func (h *Handlers) SiteCapturedMessagesDeleteAll(ctx context.Context, _ siteapi.SiteCapturedMessagesDeleteAllParams) (siteapi.SiteCapturedMessagesDeleteAllRes, error) {
	s := workspaceScope(ctx)
	if err := h.captured.DeleteAll(ctx, s); err != nil {
		return nil, err
	}
	return &siteapi.SiteCapturedMessagesDeleteAllNoContent{}, nil
}
