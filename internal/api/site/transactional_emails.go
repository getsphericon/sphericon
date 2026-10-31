package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
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

	return mapper.TransactionalEmailsListPage(page), nil
}
