package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteAttentionGet maps what needs attention in the Workspace of the path onto the DTO.
func (h *Handlers) SiteAttentionGet(ctx context.Context, _ siteapi.SiteAttentionGetParams) (siteapi.SiteAttentionGetRes, error) {
	items, err := h.attention.Items(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAttentionList{Items: mapper.AttentionItemsToResources(items)}, nil
}
