package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/suppressions"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) SiteSuppressionsList(ctx context.Context, params siteapi.SiteSuppressionsListParams) (*siteapi.SiteSuppressionsListOK, error) {
	s := workspaceScope(ctx)

	page, err := suppressions.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize), params.Lifted.Or(false))
	if err != nil {
		return nil, err
	}

	return mapper.SuppressionsListPage(page), nil
}

func (h *Handlers) SiteSuppressionsCreate(ctx context.Context, req *siteapi.SiteCreateSuppressionInput, _ siteapi.SiteSuppressionsCreateParams) (*siteapi.SiteSuppressionResource, error) {
	s := workspaceScope(ctx)

	created, err := suppressions.AddManual(ctx, s, inputs.SuppressionChannel(req.Channel.Or(siteapi.SiteSuppressionChannelEmail)), req.Destination)
	if err != nil {
		return nil, err
	}
	res := mapper.SuppressionToResource(created)
	return &res, nil
}

func (h *Handlers) SiteSuppressionsDelete(ctx context.Context, params siteapi.SiteSuppressionsDeleteParams) error {
	s := workspaceScope(ctx)

	id := int64(params.ID)
	return suppressions.Remove(ctx, s, id, h.now())
}
