package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// SiteTransactionalEmailsList returns the workspace's transactional send history
// (the Outbound messages the /api/emails surface wrote), most recent first.
func (h *Handlers) SiteTransactionalEmailsList(ctx context.Context, params siteapi.SiteTransactionalEmailsListParams) (siteapi.SiteTransactionalEmailsListRes, error) {
	s := workspaceScope(ctx)

	page, err := h.outbound.ListTransactional(ctx, s, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	res := make([]siteapi.SiteTransactionalEmailResource, len(page.Items))
	for i, t := range page.Items {
		res[i] = mapper.TransactionalMessageToResource(t)
	}
	return &siteapi.SiteTransactionalEmailsListOK{
		Items:      res,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}
