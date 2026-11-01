package staff

import (
	"context"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// ServiceWorkspaceFeedList is the Workspaces created or changed since a cursor, each
// with all of its owners (needs workspaces:read).
func (h *ServiceHandlers) ServiceWorkspaceFeedList(ctx context.Context, params serviceapi.ServiceWorkspaceFeedListParams) (serviceapi.ServiceWorkspaceFeedListRes, error) {
	page, err := h.module.WorkspaceFeed(ctx, params.Cursor.Or(""), int(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	return serviceMapper.WorkspaceFeedPage(pagination.MapCursor(page, func(e FeedEntry) serviceapi.ServiceWorkspaceResource {
		item := serviceMapper.WorkspaceToResource(e.Workspace)
		item.Owners = serviceMapper.OwnersToResources(e.Owners)
		return item
	})), nil
}
