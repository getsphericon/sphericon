package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) EnrollmentsList(ctx context.Context, params externalapi.EnrollmentsListParams) (*externalapi.EnrollmentsListOK, error) {
	page, err := h.automations.Enrollments(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), inputs.EnrollmentsFilter(params))
	if err != nil {
		return nil, err
	}
	return mapper.EnrollmentsListPage(page), nil
}

func (h *Handlers) EnrollmentsGet(ctx context.Context, params externalapi.EnrollmentsGetParams) (*externalapi.EnrollmentResource, error) {
	r, err := h.automations.Enrollment(ctx, auth.TokenScoped(ctx), int64(params.AutomationId), int64(params.EnrollmentId))
	if err != nil {
		return nil, err
	}
	res := mapper.EnrollmentToResource(r)
	return &res, nil
}
