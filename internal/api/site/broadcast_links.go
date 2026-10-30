package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

func (h *Handlers) SiteBroadcastLinksList(ctx context.Context, params siteapi.SiteBroadcastLinksListParams) (siteapi.SiteBroadcastLinksListRes, error) {
	ws := workspaceScope(ctx)
	links, err := h.broadcasts.LinkClicksOf(ctx, ws, int64(params.BroadcastId))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteBroadcastLinkClicksList{Items: mapper.LinkClicksToResource(links)}, nil
}
