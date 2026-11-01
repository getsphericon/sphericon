package staff

import (
	"context"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// ServiceUserFeedList is the Users created, changed or deleted since a cursor, each with their
// current consent to the operator's mailings (needs users:read).
func (h *ServiceHandlers) ServiceUserFeedList(ctx context.Context, params serviceapi.ServiceUserFeedListParams) (serviceapi.ServiceUserFeedListRes, error) {
	page, err := h.module.UserFeed(ctx, params.Cursor.Or(""), int(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	return serviceMapper.UserFeedPage(pagination.MapCursor(page, userFeedItem)), nil
}

// userFeedItem is a feed entry as a resource: a tombstone, or the User with the mailings consent
// the entry carries.
func userFeedItem(e UserFeedEntry) serviceapi.ServiceUserResource {
	if e.Tombstone != nil {
		return serviceMapper.TombstoneToResource(e.Tombstone)
	}
	item := serviceMapper.UserToResource(e.User)
	if e.Mailing != nil {
		item.MailingsConsent = e.Mailing.Accepted
		item.MailingsConsentChangedAt = serviceapi.NewOptNilTimestamp(serviceapi.Timestamp(e.Mailing.DecidedAt))
	}
	return item
}
