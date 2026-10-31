package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// SiteEventsList returns the workspace's events, most recent first.
func (h *Handlers) SiteEventsList(ctx context.Context, params siteapi.SiteEventsListParams) (siteapi.SiteEventsListRes, error) {
	scoped := workspaceScope(ctx)

	f := eventlog.Filter{Action: params.Action.Or(""), Email: params.Email.Or(""), IncludeMachine: params.IncludeMachine.Or(false)}
	f.ContactID = convert.Int64Ptr(params.ContactId)
	page, err := h.eventlog.List(ctx, scoped, f, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	out := mapper.EventsListPage(page)
	return &out, nil
}

// SiteEventsActions returns the distinct event actions in the workspace, sorted —
// used to populate the segment builder's event-condition picker.
func (h *Handlers) SiteEventsActions(ctx context.Context, _ siteapi.SiteEventsActionsParams) (siteapi.SiteEventsActionsRes, error) {
	scoped := workspaceScope(ctx)

	actions, err := h.eventlog.Actions(ctx, scoped)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteEventActionsResult{Actions: actions}, nil
}
