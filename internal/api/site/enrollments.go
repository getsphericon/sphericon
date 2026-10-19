package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) SiteEnrollmentsList(ctx context.Context, params siteapi.SiteEnrollmentsListParams) (siteapi.SiteEnrollmentsListRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteEnrollmentsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
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
		Items:      lo.Map(page.Items, func(r *ent.AutomationRun, _ int) siteapi.SiteEnrollmentResource { return enrollmentResource(r) }),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteEnrollmentsGet(ctx context.Context, params siteapi.SiteEnrollmentsGetParams) (siteapi.SiteEnrollmentsGetRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteEnrollmentsGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := h.automations.Enrollment(ctx, ws, int64(params.AutomationId), int64(params.EnrollmentId))
	if errors.Is(err, automations.ErrNotFound) {
		v := siteapi.SiteEnrollmentsGetNotFound(problem(http.StatusNotFound, "enrollment not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := enrollmentResource(r)
	return &res, nil
}

func enrollmentResource(r *ent.AutomationRun) siteapi.SiteEnrollmentResource {
	out := siteapi.SiteEnrollmentResource{
		ID:           siteapi.EntityId(r.ID),
		AutomationId: siteapi.EntityId(r.AutomationID),
		ContactId:    siteapi.EntityId(r.ContactID),
		Status:       siteapi.SiteEnrollmentStatus(r.Status),
		CurrentStep:  int32(r.CurrentStep),
		CreatedAt:    siteapi.Timestamp(r.CreatedAt),
		UpdatedAt:    siteapi.Timestamp(r.UpdatedAt),
	}
	if r.ResumeAt != nil {
		out.ResumeAt = siteapi.NewOptTimestamp(siteapi.Timestamp(*r.ResumeAt))
	}
	if r.FailureKind != nil {
		f := siteapi.SiteEnrollmentFailure{Kind: siteapi.SiteEnrollmentFailureKind(*r.FailureKind)}
		if r.FailureStatus != nil {
			f.Status = siteapi.NewOptInt32(int32(*r.FailureStatus))
		}
		if r.FailureExcerpt != nil {
			f.Excerpt = siteapi.NewOptString(*r.FailureExcerpt)
		}
		out.Failure = siteapi.NewOptSiteEnrollmentFailure(f)
	}
	return out
}
