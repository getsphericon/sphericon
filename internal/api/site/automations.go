package site

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/samber/lo"
)

func (h *Handlers) SiteAutomationsList(ctx context.Context, params siteapi.SiteAutomationsListParams) (siteapi.SiteAutomationsListRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAutomationsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.automations.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteAutomationsListOK{
		Items:      lo.Map(page.Items, func(a *ent.Automation, _ int) siteapi.SiteAutomationResource { return mapper.AutomationToResource(a) }),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

func (h *Handlers) SiteAutomationsCreate(ctx context.Context, req *siteapi.SiteCreateAutomationInput, params siteapi.SiteAutomationsCreateParams) (siteapi.SiteAutomationsCreateRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAutomationsCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	in := automations.CreateInput{Name: req.Name, TriggerEvent: req.TriggerEvent}
	if req.Steps != nil {
		in.Steps = resources.AutomationSteps(req.Steps)
	}
	a, err := h.automations.Create(ctx, ws, in)
	if errors.Is(err, automations.ErrInvalidStep) {
		v := siteapi.SiteAutomationsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsGet(ctx context.Context, params siteapi.SiteAutomationsGetParams) (siteapi.SiteAutomationsGetRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAutomationsGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteAutomationsGetBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	a, err := h.automations.Get(ctx, ws, id)
	if errors.Is(err, automations.ErrNotFound) {
		v := siteapi.SiteAutomationsGetNotFound(problem(http.StatusNotFound, "automation not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsUpdate(ctx context.Context, req *siteapi.SiteUpdateAutomationInput, params siteapi.SiteAutomationsUpdateParams) (siteapi.SiteAutomationsUpdateRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAutomationsUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteAutomationsUpdateBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	in := automations.UpdateInput{
		Name:         convert.StringPtr(req.Name),
		TriggerEvent: convert.StringPtr(req.TriggerEvent),
	}
	if req.Steps != nil {
		steps := resources.AutomationSteps(req.Steps)
		in.Steps = &steps
	}
	a, err := h.automations.Update(ctx, ws, id, in)
	if errors.Is(err, automations.ErrNotFound) {
		v := siteapi.SiteAutomationsUpdateNotFound(problem(http.StatusNotFound, "automation not found"))
		return &v, nil
	}
	if errors.Is(err, automations.ErrInvalidStep) {
		v := siteapi.SiteAutomationsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsDelete(ctx context.Context, params siteapi.SiteAutomationsDeleteParams) (siteapi.SiteAutomationsDeleteRes, error) {
	ws, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteAutomationsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteAutomationsDeleteBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	err = h.automations.Delete(ctx, ws, id)
	if errors.Is(err, automations.ErrNotFound) {
		v := siteapi.SiteAutomationsDeleteNotFound(problem(http.StatusNotFound, "automation not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteAutomationsDeleteNoContent{}, nil
}

func (h *Handlers) SiteAutomationsActivate(ctx context.Context, params siteapi.SiteAutomationsActivateParams) (siteapi.SiteAutomationsActivateRes, error) {
	a, err := h.setAutomationActive(ctx, params.Slug, params.ID, true)
	if errors.Is(err, automations.ErrNotFound) || ent.IsNotFound(err) {
		v := siteapi.SiteAutomationsActivateNotFound(problem(http.StatusNotFound, "automation not found"))
		return &v, nil
	}
	if err != nil {
		v := siteapi.SiteAutomationsActivateBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) SiteAutomationsDeactivate(ctx context.Context, params siteapi.SiteAutomationsDeactivateParams) (siteapi.SiteAutomationsDeactivateRes, error) {
	a, err := h.setAutomationActive(ctx, params.Slug, params.ID, false)
	if errors.Is(err, automations.ErrNotFound) || ent.IsNotFound(err) {
		v := siteapi.SiteAutomationsDeactivateNotFound(problem(http.StatusNotFound, "automation not found"))
		return &v, nil
	}
	if err != nil {
		v := siteapi.SiteAutomationsDeactivateBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) setAutomationActive(ctx context.Context, slug string, id siteapi.EntityId, active bool) (*ent.Automation, error) {
	ws, err := h.scopedFor(ctx, slug)
	if err != nil {
		return nil, err
	}
	parsed, err := strconv.ParseInt(string(id), 10, 64)
	if err != nil {
		return nil, err
	}
	if active {
		return h.automations.Activate(ctx, ws, parsed)
	}
	return h.automations.Deactivate(ctx, ws, parsed)
}
