package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) EnrollmentsList(ctx context.Context, params externalapi.EnrollmentsListParams) (externalapi.EnrollmentsListRes, error) {
	status := ""
	if v, ok := params.Status.Get(); ok {
		status = string(v)
	}
	page, err := h.automations.Enrollments(ctx, auth.TokenScoped(ctx), int64(params.AutomationId), status, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	out := mapper.EnrollmentsListPage(page)
	return &out, nil
}

func (h *Handlers) EnrollmentsGet(ctx context.Context, params externalapi.EnrollmentsGetParams) (externalapi.EnrollmentsGetRes, error) {
	r, err := h.automations.Enrollment(ctx, auth.TokenScoped(ctx), int64(params.AutomationId), int64(params.EnrollmentId))
	if err != nil {
		return nil, err
	}
	res := mapper.EnrollmentToResource(r)
	return &res, nil
}
