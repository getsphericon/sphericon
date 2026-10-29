package site

import (
	"context"
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/broadcasts"
)

func (h *Handlers) SiteBroadcastLinksList(ctx context.Context, params siteapi.SiteBroadcastLinksListParams) (siteapi.SiteBroadcastLinksListRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	links, err := h.broadcasts.LinkClicksOf(ctx, ws, int64(params.BroadcastId))
	if errors.Is(err, broadcasts.ErrNotFound) {
		v := siteapi.SiteBroadcastLinksListNotFound(problem(http.StatusNotFound, "broadcast not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteBroadcastLinkClicksList{Items: mapper.LinkClicksToResource(links)}, nil
}
