package staff

import (
	"context"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// ServiceTrackingHostFeedList is the Tracking domain hosts that entered or left the
// verified states since a cursor (needs domains:read).
func (h *ServiceHandlers) ServiceTrackingHostFeedList(ctx context.Context, params serviceapi.ServiceTrackingHostFeedListParams) (serviceapi.ServiceTrackingHostFeedListRes, error) {
	page, err := h.module.TrackingHostFeed(ctx, params.Cursor.Or(""), int(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	return serviceMapper.TrackingHostFeedPage(pagination.MapCursor(page, func(e HostChange) serviceapi.ServiceTrackingHostChange {
		item := serviceMapper.TrackingHostChangeToResource(e.Change)
		item.WorkspaceId = serviceapi.WorkspaceId(e.WorkspaceID)
		return item
	})), nil
}
