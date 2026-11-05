package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteEnrollmentsList(ctx context.Context, params siteapi.SiteEnrollmentsListParams) (*siteapi.SiteEnrollmentsListOK, error) {
	ws := workspaceScope(ctx)
	status := ""
	if v, ok := params.Status.Get(); ok {
		status = string(v)
	}
	page, err := h.automations.Enrollments(ctx, ws, int64(params.AutomationId), status, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.EnrollmentsListPage(page), nil
}

func (h *Handlers) SiteEnrollmentsGet(ctx context.Context, params siteapi.SiteEnrollmentsGetParams) (*siteapi.SiteEnrollmentResource, error) {
	ws := workspaceScope(ctx)
	r, err := h.automations.Enrollment(ctx, ws, int64(params.AutomationId), int64(params.EnrollmentId))
	if err != nil {
		return nil, err
	}
	res := mapper.EnrollmentToResource(r)
	return &res, nil
}
