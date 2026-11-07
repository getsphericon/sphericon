package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/outboundmessages"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// SiteEventsList returns the workspace's events, most recent first.
func (h *Handlers) SiteEventsList(ctx context.Context, params siteapi.SiteEventsListParams) (*siteapi.SiteEventsListOK, error) {
	scoped := workspaceScope(ctx)

	f := events.LogFilter{Action: params.Action.Or(""), Email: params.Email.Or(""), IncludeMachine: params.IncludeMachine.Or(false)}
	f.ContactID = convert.Int64Ptr(params.ContactId)
	page, err := events.ListLog(ctx, scoped, f, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	subjects, err := outboundmessages.SubjectsOf(ctx, scoped, page.Items)
	if err != nil {
		return nil, err
	}
	res := mapper.EventsListPage(page)
	// The page and its items are in the same order.
	for i, e := range page.Items {
		if id, ok := events.OutboundMessageID(e); ok {
			if subject, kept := subjects[id]; kept {
				res.Items[i].EmailSubject = siteapi.NewOptNilString(subject)
			}
		}
	}
	return res, nil
}

// SiteEventsActions returns the distinct event actions in the workspace, sorted —
// used to populate the segment builder's event-condition picker.
func (h *Handlers) SiteEventsActions(ctx context.Context, _ siteapi.SiteEventsActionsParams) (*siteapi.SiteEventActionsResult, error) {
	scoped := workspaceScope(ctx)

	actions, err := events.DistinctActions(ctx, scoped)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteEventActionsResult{Actions: actions}, nil
}
