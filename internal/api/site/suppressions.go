package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/suppression"
)

func (h *Handlers) SiteSuppressionsList(ctx context.Context, params siteapi.SiteSuppressionsListParams) (siteapi.SiteSuppressionsListRes, error) {
	s := workspaceScope(ctx)

	page, err := suppression.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize), params.Lifted.Or(false))
	if err != nil {
		return nil, err
	}

	return mapper.SuppressionsListPage(page), nil
}

func (h *Handlers) SiteSuppressionsCreate(ctx context.Context, req *siteapi.SiteCreateSuppressionInput, _ siteapi.SiteSuppressionsCreateParams) (siteapi.SiteSuppressionsCreateRes, error) {
	s := workspaceScope(ctx)

	created, err := suppression.AddManual(ctx, s, req.Destination)
	if err != nil {
		return nil, err
	}
	res := mapper.SuppressionToResource(created)
	return &res, nil
}

func (h *Handlers) SiteSuppressionsDelete(ctx context.Context, params siteapi.SiteSuppressionsDeleteParams) (siteapi.SiteSuppressionsDeleteRes, error) {
	s := workspaceScope(ctx)

	id := int64(params.ID)
	if err := suppression.Remove(ctx, s, id); err != nil {
		return nil, err
	}
	return &siteapi.SiteSuppressionsDeleteNoContent{}, nil
}
