package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteEnrollmentsList(ctx context.Context, params siteapi.SiteEnrollmentsListParams) (siteapi.SiteEnrollmentsListRes, error) {
	ws := workspaceScope(ctx)
	status := ""
	if v, ok := params.Status.Get(); ok {
		status = string(v)
	}
	page, err := h.automations.Enrollments(ctx, ws, int64(params.AutomationId), status, pagination.ParamsOf(params.Page, params.PageSize))
	if errors.Is(err, automations.ErrNotFound) {
		v := siteapi.SiteEnrollmentsListNotFound(problem(http.StatusNotFound, "automation not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteEnrollmentsListOK{
		Items: lo.Map(page.Items, func(r *ent.AutomationRun, _ int) siteapi.SiteEnrollmentResource {
			return mapper.EnrollmentToResource(r)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteEnrollmentsGet(ctx context.Context, params siteapi.SiteEnrollmentsGetParams) (siteapi.SiteEnrollmentsGetRes, error) {
	ws := workspaceScope(ctx)
	r, err := h.automations.Enrollment(ctx, ws, int64(params.AutomationId), int64(params.EnrollmentId))
	if errors.Is(err, automations.ErrNotFound) {
		v := siteapi.SiteEnrollmentsGetNotFound(problem(http.StatusNotFound, "enrollment not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.EnrollmentToResource(r)
	return &res, nil
}
