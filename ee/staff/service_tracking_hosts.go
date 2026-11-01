package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
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
	ids := make([]int64, len(page.Items))
	for i, e := range page.Items {
		ids[i] = e.WorkspaceID
	}
	publicIDs, err := h.module.publicIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return serviceMapper.TrackingHostFeedPage(pagination.MapCursor(page, func(e *ent.TrackingHostChange) serviceapi.ServiceTrackingHostChange {
		item := serviceMapper.TrackingHostChangeToResource(e)
		item.WorkspaceId = serviceapi.WorkspaceId(publicIDs[e.WorkspaceID])
		return item
	})), nil
}
