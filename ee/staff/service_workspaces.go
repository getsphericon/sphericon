package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/samber/lo"
)

// ServiceWorkspaceFeedList is the Workspaces created or changed since a cursor, each
// with all of its owners (needs workspaces:read).
func (h *ServiceHandlers) ServiceWorkspaceFeedList(ctx context.Context, params serviceapi.ServiceWorkspaceFeedListParams) (serviceapi.ServiceWorkspaceFeedListRes, error) {
	page, err := h.module.WorkspaceFeed(ctx, params.Cursor.Or(""), int(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	out := &serviceapi.ServiceWorkspaceFeedPage{
		Items:      make([]serviceapi.ServiceWorkspaceResource, len(page.Entries)),
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
	for i, e := range page.Entries {
		item := serviceMapper.WorkspaceToResource(e.Workspace)
		item.Owners = lo.Map(e.Owners, func(u *ent.User, _ int) serviceapi.ServiceWorkspaceOwner { return serviceMapper.OwnerToResource(u) })
		out.Items[i] = item
	}
	return out, nil
}
