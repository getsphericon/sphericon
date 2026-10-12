package site

import (
	"context"
	"net/http"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// SiteEventsList returns the workspace's events, most recent first.
func (h *Handlers) SiteEventsList(ctx context.Context, params siteapi.SiteEventsListParams) (siteapi.SiteEventsListRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	f := eventlog.Filter{Action: params.Action.Or(""), Email: params.Email.Or("")}
	if v, ok := params.ContactId.Get(); ok {
		if id, perr := strconv.ParseInt(string(v), 10, 64); perr == nil {
			f.ContactID = &id
		}
	}
	page, err := h.eventlog.List(ctx, scoped, f, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	resources := make([]siteapi.SiteEventResource, len(page.Items))
	for i, e := range page.Items {
		resources[i] = mapper.EventToResource(e)
	}

	return &siteapi.SiteEventsListOK{
		Items:      resources,
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

// SiteEventsActions returns the distinct event actions in the workspace, sorted —
// used to populate the segment builder's event-condition picker.
func (h *Handlers) SiteEventsActions(ctx context.Context, params siteapi.SiteEventsActionsParams) (siteapi.SiteEventsActionsRes, error) {
	scoped, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	actions, err := h.eventlog.Actions(ctx, scoped)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteEventActionsResult{Actions: actions}, nil
}
