package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// ServiceWorkspaceFeedList is the Workspaces created or changed since a cursor, each
// with all of its owners (needs workspaces:read).
func (h *ServiceHandlers) ServiceWorkspaceFeedList(ctx context.Context, params serviceapi.ServiceWorkspaceFeedListParams) (*serviceapi.ServiceWorkspaceFeedPage, error) {
	page, err := h.module.WorkspaceFeed(ctx, params.Cursor.Or(""), int(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	return serviceMapper.WorkspaceFeedPage(pagination.MapCursor(page, func(e workspaceoversight.FeedEntry) serviceapi.ServiceWorkspaceResource {
		item := serviceMapper.WorkspaceToResource(e.Workspace)
		item.Owners = serviceMapper.OwnersToResources(e.Owners)
		return item
	})), nil
}
