package staff

import (
	"context"
	"errors"
	"net/http"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/sendingdomains"
)

// ServiceTrackingHostFeedList is the Tracking domain hosts that entered or left the
// verified states since a cursor (needs domains:read).
func (h *ServiceHandlers) ServiceTrackingHostFeedList(ctx context.Context, params serviceapi.ServiceTrackingHostFeedListParams) (serviceapi.ServiceTrackingHostFeedListRes, error) {
	page, err := h.module.TrackingHostFeed(ctx, params.Cursor.Or(""), int(params.Limit.Or(0)))
	if errors.Is(err, ErrInvalidFeedCursor) {
		return &serviceapi.ServiceTrackingHostFeedListBadRequest{
			Status: serviceapi.NewOptInt32(http.StatusBadRequest),
			Title:  serviceapi.NewOptString(http.StatusText(http.StatusBadRequest)),
			Detail: serviceapi.NewOptString(err.Error()),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	out := &serviceapi.ServiceTrackingHostFeedPage{
		Items:      make([]serviceapi.ServiceTrackingHostChange, len(page.Entries)),
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
	ids := make([]int64, len(page.Entries))
	for i, e := range page.Entries {
		ids[i] = e.WorkspaceID
	}
	publicIDs, err := h.module.publicIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i, e := range page.Entries {
		out.Items[i] = serviceapi.ServiceTrackingHostChange{
			Host:        sendingdomains.TrackingHost(e.Domain),
			WorkspaceId: serviceapi.WorkspaceId(publicIDs[e.WorkspaceID]),
			State:       serviceapi.ServiceTrackingHostState(e.State),
			ChangedAt:   serviceapi.Timestamp(e.CreatedAt),
		}
	}
	return out, nil
}
