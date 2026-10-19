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

const scopeEnrollmentsRead = "enrollments:read"

func (h *Handlers) EnrollmentsList(ctx context.Context, params externalapi.EnrollmentsListParams) (externalapi.EnrollmentsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeEnrollmentsRead) {
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
		Items:      lo.Map(page.Items, func(r *ent.AutomationRun, _ int) externalapi.EnrollmentResource { return enrollmentResource(r) }),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) EnrollmentsGet(ctx context.Context, params externalapi.EnrollmentsGetParams) (externalapi.EnrollmentsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeEnrollmentsRead) {
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
	res := enrollmentResource(r)
	return &res, nil
}

func enrollmentResource(r *ent.AutomationRun) externalapi.EnrollmentResource {
	out := externalapi.EnrollmentResource{
		ID:           externalapi.EntityId(r.ID),
		AutomationId: externalapi.EntityId(r.AutomationID),
		ContactId:    externalapi.EntityId(r.ContactID),
		Status:       externalapi.EnrollmentStatus(r.Status),
		CurrentStep:  int32(r.CurrentStep),
		CreatedAt:    externalapi.Timestamp(r.CreatedAt),
		UpdatedAt:    externalapi.Timestamp(r.UpdatedAt),
	}
	if r.ResumeAt != nil {
		out.ResumeAt = externalapi.NewOptTimestamp(externalapi.Timestamp(*r.ResumeAt))
	}
	if r.FailureKind != nil {
		f := externalapi.EnrollmentFailure{Kind: externalapi.EnrollmentFailureKind(*r.FailureKind)}
		if r.FailureStatus != nil {
			f.Status = externalapi.NewOptInt32(int32(*r.FailureStatus))
		}
		if r.FailureExcerpt != nil {
			f.Excerpt = externalapi.NewOptString(*r.FailureExcerpt)
		}
		out.Failure = externalapi.NewOptEnrollmentFailure(f)
	}
	return out
}
