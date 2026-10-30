package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/apitokens"
)

// SiteConnectionsList returns the Workspace's active Connections the caller may see: their own,
// or every member's with members:manage (the Policy of apitokens.Connections).
func (h *Handlers) SiteConnectionsList(ctx context.Context, _ siteapi.SiteConnectionsListParams) (siteapi.SiteConnectionsListRes, error) {
	connections, err := apitokens.Connections(ctx, workspaceScope(ctx), callerMembership(ctx))
	if err != nil {
		return nil, err
	}
	items := make(siteapi.SiteConnectionsListOKApplicationJSON, len(connections))
	for i, c := range connections {
		items[i] = mapper.ConnectionToResource(c)
	}
	return &items, nil
}

// SiteConnectionsDelete revokes a Connection: the caller's own, or anyone's with members:manage.
func (h *Handlers) SiteConnectionsDelete(ctx context.Context, params siteapi.SiteConnectionsDeleteParams) (siteapi.SiteConnectionsDeleteRes, error) {
	if err := apitokens.RevokeConnection(ctx, workspaceScope(ctx), callerMembership(ctx), int64(params.ID)); err != nil {
		return nil, err
	}
	return &siteapi.SiteConnectionsDeleteNoContent{}, nil
}
