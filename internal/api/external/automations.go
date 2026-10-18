package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/api/external/resources"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/samber/lo"
)

const (
	scopeAutomationsRead     = "automations:read"
	scopeAutomationsWrite    = "automations:write"
	scopeAutomationsActivate = "automations:activate"
)

func (h *Handlers) AutomationsList(ctx context.Context, params externalapi.AutomationsListParams) (externalapi.AutomationsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeAutomationsRead) {
		res := externalapi.AutomationsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.automations.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &externalapi.AutomationsListOK{
		Items:      lo.Map(page.Items, func(a *ent.Automation, _ int) externalapi.AutomationResource { return mapper.AutomationToResource(a) }),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

// AutomationsCreate always stores a draft: activating is the send-class operation.
func (h *Handlers) AutomationsCreate(ctx context.Context, req *externalapi.CreateAutomationInput) (externalapi.AutomationsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeAutomationsWrite) {
		res := externalapi.AutomationsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	in := automations.CreateInput{Name: req.Name, TriggerEvent: req.TriggerEvent}
	if req.Steps != nil {
		in.Steps = resources.AutomationSteps(req.Steps)
	}
	a, err := h.automations.Create(ctx, auth.TokenScoped(ctx), in)
	if errors.Is(err, automations.ErrInvalidStep) {
		res := externalapi.AutomationsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsGet(ctx context.Context, params externalapi.AutomationsGetParams) (externalapi.AutomationsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeAutomationsRead) {
		res := externalapi.AutomationsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	a, err := h.automations.Get(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsGetNotFound(problem(http.StatusNotFound, "automation not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsUpdate(ctx context.Context, req *externalapi.UpdateAutomationInput, params externalapi.AutomationsUpdateParams) (externalapi.AutomationsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeAutomationsWrite) {
		res := externalapi.AutomationsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)

	in := automations.UpdateInput{
		Name:         convert.StringPtr(req.Name),
		TriggerEvent: convert.StringPtr(req.TriggerEvent),
	}
	if req.Steps != nil {
		steps := resources.AutomationSteps(req.Steps)
		in.Steps = &steps
	}
	a, err := h.automations.Update(ctx, auth.TokenScoped(ctx), id, in)
	switch {
	case errors.Is(err, automations.ErrNotFound):
		res := externalapi.AutomationsUpdateNotFound(problem(http.StatusNotFound, "automation not found"))
		return &res, nil
	case errors.Is(err, automations.ErrInvalidStep):
		res := externalapi.AutomationsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsDelete(ctx context.Context, params externalapi.AutomationsDeleteParams) (externalapi.AutomationsDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeAutomationsWrite) {
		res := externalapi.AutomationsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	err := h.automations.Delete(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsDeleteNotFound(problem(http.StatusNotFound, "automation not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.AutomationsDeleteNoContent{}, nil
}

// AutomationsActivate and AutomationsDeactivate need automations:activate, which
// authoring scopes never imply (ADR 0016). Over MCP, activate additionally needs
// mcp:send (the x-mcp send flag).
func (h *Handlers) AutomationsActivate(ctx context.Context, params externalapi.AutomationsActivateParams) (externalapi.AutomationsActivateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeAutomationsActivate) {
		res := externalapi.AutomationsActivateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	a, err := h.automations.Activate(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsActivateNotFound(problem(http.StatusNotFound, "automation not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsDeactivate(ctx context.Context, params externalapi.AutomationsDeactivateParams) (externalapi.AutomationsDeactivateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), scopeAutomationsActivate) {
		res := externalapi.AutomationsDeactivateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	a, err := h.automations.Deactivate(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsDeactivateNotFound(problem(http.StatusNotFound, "automation not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}
