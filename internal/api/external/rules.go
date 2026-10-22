package external

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/api/external/resources"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/rules"
)

// Rules reuse the Automation scopes: a Rule is the light form of an Automation, so reading
// and authoring take automations:read and automations:write, and activating takes the
// send-class automations:activate (ADR 0016).

func (h *Handlers) RulesList(ctx context.Context, params externalapi.RulesListParams) (externalapi.RulesListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsRead) {
		res := externalapi.RulesListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.rules.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &externalapi.RulesListOK{
		Items:      lo.Map(page.Items, func(r *ent.Rule, _ int) externalapi.RuleResource { return mapper.RuleToResource(r) }),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

// RulesCreate always stores a draft: activating is the send-class operation.
func (h *Handlers) RulesCreate(ctx context.Context, req *externalapi.CreateRuleInput) (externalapi.RulesCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsWrite) {
		res := externalapi.RulesCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	in := rules.CreateInput{
		Name:          req.Name,
		TriggerEvents: req.TriggerEvents,
		Actions:       resources.RuleActions(req.Actions),
	}
	if group, ok := req.Condition.Get(); ok {
		def, err := condition.Encode(&group)
		if err != nil {
			return nil, err
		}
		in.Condition = &def
	}
	r, err := h.rules.Create(ctx, auth.TokenScoped(ctx), in)
	if errors.Is(err, rules.ErrInvalid) {
		res := externalapi.RulesCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) RulesGet(ctx context.Context, params externalapi.RulesGetParams) (externalapi.RulesGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsRead) {
		res := externalapi.RulesGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	r, err := h.rules.Get(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if errors.Is(err, rules.ErrNotFound) {
		res := externalapi.RulesGetNotFound(problem(http.StatusNotFound, "rule not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) RulesUpdate(ctx context.Context, req *externalapi.UpdateRuleInput, params externalapi.RulesUpdateParams) (externalapi.RulesUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsWrite) {
		res := externalapi.RulesUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	in := rules.UpdateInput{Name: convert.StringPtr(req.Name)}
	if req.TriggerEvents != nil {
		in.TriggerEvents = &req.TriggerEvents
	}
	if req.Actions != nil {
		actions := resources.RuleActions(req.Actions)
		in.Actions = &actions
	}
	if group, ok := req.Condition.Get(); ok {
		def, err := condition.Encode(&group)
		if err != nil {
			return nil, err
		}
		in.Condition = &def
	}
	r, err := h.rules.Update(ctx, auth.TokenScoped(ctx), int64(params.ID), in)
	switch {
	case errors.Is(err, rules.ErrNotFound):
		res := externalapi.RulesUpdateNotFound(problem(http.StatusNotFound, "rule not found"))
		return &res, nil
	case errors.Is(err, rules.ErrInvalid):
		res := externalapi.RulesUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesActivate and RulesDeactivate need automations:activate, which authoring scopes never
// imply (ADR 0016). Over MCP, activate additionally needs mcp:send (the x-mcp send flag).
func (h *Handlers) RulesActivate(ctx context.Context, params externalapi.RulesActivateParams) (externalapi.RulesActivateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsActivate) {
		res := externalapi.RulesActivateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	r, err := h.rules.Activate(ctx, auth.TokenScoped(ctx), int64(params.ID))
	switch {
	case errors.Is(err, rules.ErrNotFound):
		res := externalapi.RulesActivateNotFound(problem(http.StatusNotFound, "rule not found"))
		return &res, nil
	case errors.Is(err, rules.ErrInvalid):
		res := externalapi.RulesActivateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) RulesDeactivate(ctx context.Context, params externalapi.RulesDeactivateParams) (externalapi.RulesDeactivateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsActivate) {
		res := externalapi.RulesDeactivateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	r, err := h.rules.Deactivate(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if errors.Is(err, rules.ErrNotFound) {
		res := externalapi.RulesDeactivateNotFound(problem(http.StatusNotFound, "rule not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RuleFiringsList lists a Rule's Firings, newest first.
func (h *Handlers) RuleFiringsList(ctx context.Context, params externalapi.RuleFiringsListParams) (externalapi.RuleFiringsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsRead) {
		res := externalapi.RuleFiringsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	page, err := h.rules.Firings(ctx, auth.TokenScoped(ctx), int64(params.RuleId), pagination.ParamsOf(params.Page, params.PageSize))
	if errors.Is(err, rules.ErrNotFound) {
		res := externalapi.RuleFiringsListNotFound(problem(http.StatusNotFound, "rule not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.RuleFiringsListOK{
		Items: lo.Map(page.Items, func(f *ent.RuleFiring, _ int) externalapi.RuleFiringResource {
			return mapper.RuleFiringToResource(f)
		}),
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}
