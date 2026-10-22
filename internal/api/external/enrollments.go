package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) EnrollmentsList(ctx context.Context, params externalapi.EnrollmentsListParams) (externalapi.EnrollmentsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeEnrollmentsRead) {
		res := externalapi.EnrollmentsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	status := ""
	if v, ok := params.Status.Get(); ok {
		status = string(v)
	}
	page, err := h.automations.Enrollments(ctx, auth.TokenScoped(ctx), int64(params.AutomationId), status, pagination.ParamsOf(params.Page, params.PageSize))
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.EnrollmentsListNotFound(problem(http.StatusNotFound, "automation not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.EnrollmentsListOK{
		Items: lo.Map(page.Items, func(r *ent.AutomationRun, _ int) externalapi.EnrollmentResource {
			return mapper.EnrollmentToResource(r)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) EnrollmentsGet(ctx context.Context, params externalapi.EnrollmentsGetParams) (externalapi.EnrollmentsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeEnrollmentsRead) {
		res := externalapi.EnrollmentsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	r, err := h.automations.Enrollment(ctx, auth.TokenScoped(ctx), int64(params.AutomationId), int64(params.EnrollmentId))
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.EnrollmentsGetNotFound(problem(http.StatusNotFound, "enrollment not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.EnrollmentToResource(r)
	return &res, nil
}
