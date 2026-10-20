package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/suppression"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/consent"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteSuppressionsList(ctx context.Context, params siteapi.SiteSuppressionsListParams) (siteapi.SiteSuppressionsListRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSuppressionsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	var pagePtr, pageSizePtr *int32
	if v, ok := params.Page.Get(); ok {
		pagePtr = &v
	}
	if v, ok := params.PageSize.Get(); ok {
		pageSizePtr = &v
	}
	page, pageSize := pagination.Normalize(pagePtr, pageSizePtr)

	q := s.Suppression().Query()
	total, err := q.Count(ctx)
	if err != nil {
		return nil, err
	}
	items, err := q.Order(ent.Desc(suppression.FieldID)).
		Limit(pageSize).
		Offset(pagination.Offset(page, pageSize)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	resources := make([]siteapi.SiteSuppressionResource, len(items))
	for i, item := range items {
		resources[i] = mapper.SuppressionToResource(item)
	}
	return &siteapi.SiteSuppressionsListOK{
		Items:      resources,
		Page:       int32(page),
		PageSize:   int32(pageSize),
		TotalItems: int32(total),
		TotalPages: int32(pagination.TotalPages(total, pageSize)),
	}, nil
}

func (h *Handlers) SiteSuppressionsCreate(ctx context.Context, req *siteapi.SiteCreateSuppressionInput, params siteapi.SiteSuppressionsCreateParams) (siteapi.SiteSuppressionsCreateRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSuppressionsCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	created, err := consent.Suppress(ctx, s, req.Destination)
	if errors.Is(err, consent.ErrDestinationEmpty) {
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
	if ent.IsNotFound(err) {
		v := siteapi.SiteSuppressionsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = s.Suppression().DeleteOneID(id).Exec(ctx)
	if ent.IsNotFound(err) {
		v := siteapi.SiteSuppressionsDeleteNotFound(problem(http.StatusNotFound, "suppression not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSuppressionsDeleteNoContent{}, nil
}
