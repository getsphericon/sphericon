package site

import (
	"context"
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/suppression"
)

func (h *Handlers) SiteSuppressionsList(ctx context.Context, params siteapi.SiteSuppressionsListParams) (siteapi.SiteSuppressionsListRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	page, err := suppression.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize), params.Lifted.Or(false))
	if err != nil {
		return nil, err
	}

	resources := make([]siteapi.SiteSuppressionResource, len(page.Items))
	for i, item := range page.Items {
		resources[i] = mapper.SuppressionToResource(item)
	}
	return &siteapi.SiteSuppressionsListOK{
		Items:      resources,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteSuppressionsCreate(ctx context.Context, req *siteapi.SiteCreateSuppressionInput, params siteapi.SiteSuppressionsCreateParams) (siteapi.SiteSuppressionsCreateRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	created, err := suppression.AddManual(ctx, s, req.Destination)
	if errors.Is(err, suppression.ErrDestinationEmpty) {
		v := siteapi.SiteSuppressionsCreateUnprocessableEntity(problemWithErrors(http.StatusUnprocessableEntity, i18n.T("errors.destination_invalid", nil), map[string][]string{
			"destination": {i18n.T("errors.must_not_be_empty", nil)},
		}))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.SuppressionToResource(created)
	return &res, nil
}

func (h *Handlers) SiteSuppressionsDelete(ctx context.Context, params siteapi.SiteSuppressionsDeleteParams) (siteapi.SiteSuppressionsDeleteRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = suppression.Remove(ctx, s, id)
	if errors.Is(err, suppression.ErrNotFound) {
		v := siteapi.SiteSuppressionsDeleteNotFound(problem(http.StatusNotFound, "suppression not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSuppressionsDeleteNoContent{}, nil
}
