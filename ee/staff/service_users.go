package staff

import (
	"context"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
)

// ServiceUserFeedList is the Users created, changed or deleted since a cursor, each with their
// current consent to the operator's mailings (needs users:read).
func (h *ServiceHandlers) ServiceUserFeedList(ctx context.Context, params serviceapi.ServiceUserFeedListParams) (serviceapi.ServiceUserFeedListRes, error) {
	page, err := h.module.UserFeed(ctx, params.Cursor.Or(""), int(params.Limit.Value))
	if err != nil {
		return nil, err
	}
	out := &serviceapi.ServiceUserFeedPage{
		Items:      make([]serviceapi.ServiceUserResource, len(page.Entries)),
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}
	for i, e := range page.Entries {
		if e.Tombstone != nil {
			// A feed entry is a User or a tombstone, each with its own method: no list method fits.
			// ast-grep-ignore: no-hand-mapped-list
			out.Items[i] = serviceMapper.TombstoneToResource(e.Tombstone)
			continue
		}
		item := serviceMapper.UserToResource(e.User)
		if e.Mailing != nil {
			item.MailingsConsent = e.Mailing.Accepted
			item.MailingsConsentChangedAt = serviceapi.NewOptNilTimestamp(serviceapi.Timestamp(e.Mailing.DecidedAt))
		}
		out.Items[i] = item
	}
	return out, nil
}
