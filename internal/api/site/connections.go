package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/identity/apitokens"
)

// SiteConnectionsList returns the Workspace's active Connections the caller may see: their own,
// or every member's with members:manage (the Policy of apitokens.Connections).
func (h *Handlers) SiteConnectionsList(ctx context.Context, _ siteapi.SiteConnectionsListParams) ([]siteapi.SiteConnectionResource, error) {
	connections, err := apitokens.Connections(ctx, workspaceScope(ctx), callerMembership(ctx))
	if err != nil {
		return nil, err
	}
	items := mapper.ConnectionsToResources(connections)
	return items, nil
}

// SiteConnectionsDelete revokes a Connection: the caller's own, or anyone's with members:manage.
func (h *Handlers) SiteConnectionsDelete(ctx context.Context, params siteapi.SiteConnectionsDeleteParams) error {
	return apitokens.RevokeConnection(ctx, workspaceScope(ctx), callerMembership(ctx), int64(params.ID), h.now())
}
