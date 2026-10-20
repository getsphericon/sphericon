package staff

import (
	"context"
	"errors"
	"net/http"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
)

// ServiceWorkspaceFeedList is the Workspaces created or changed since a cursor, each
// with all of its owners (needs workspaces:read).
func (h *ServiceHandlers) ServiceWorkspaceFeedList(ctx context.Context, params serviceapi.ServiceWorkspaceFeedListParams) (serviceapi.ServiceWorkspaceFeedListRes, error) {
	page, err := h.module.WorkspaceFeed(ctx, params.Cursor.Or(""), int(params.Limit.Or(0)))
	if errors.Is(err, ErrInvalidFeedCursor) {
		return &serviceapi.ServiceWorkspaceFeedListBadRequest{
			Status: serviceapi.NewOptInt32(http.StatusBadRequest),
			Title:  serviceapi.NewOptString(http.StatusText(http.StatusBadRequest)),
			Detail: serviceapi.NewOptString(err.Error()),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	out := &serviceapi.ServiceWorkspaceFeedPage{
		Items:      make([]serviceapi.ServiceWorkspaceResource, len(page.Entries)),
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
	for i, e := range page.Entries {
		item := serviceapi.ServiceWorkspaceResource{
			ID:        serviceapi.WorkspaceId(e.Workspace.PublicID),
			Name:      e.Workspace.Name,
			CreatedAt: serviceapi.Timestamp(e.Workspace.CreatedAt),
			Owners:    make([]serviceapi.ServiceWorkspaceOwner, len(e.Owners)),
		}
		if t := e.Workspace.DiscardedAt; t != nil {
			item.DeletedAt = serviceapi.NewOptNilTimestamp(serviceapi.Timestamp(*t))
		}
		if t := e.Workspace.ErasedAt; t != nil {
			item.ErasedAt = serviceapi.NewOptNilTimestamp(serviceapi.Timestamp(*t))
		}
		for j, u := range e.Owners {
			owner := serviceapi.ServiceWorkspaceOwner{
				UserId: serviceapi.EntityId(u.ID),
				Email:  serviceapi.EmailAddress(u.Email),
			}
			if u.EmailVerifiedAt != nil {
				owner.EmailVerifiedAt = serviceapi.NewOptNilTimestamp(serviceapi.Timestamp(*u.EmailVerifiedAt))
			}
			item.Owners[j] = owner
		}
		out.Items[i] = item
	}
	return out, nil
}
