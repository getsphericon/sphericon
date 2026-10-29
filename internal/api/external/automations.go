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
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/validation"
)

func (h *Handlers) AutomationsList(ctx context.Context, params externalapi.AutomationsListParams) (externalapi.AutomationsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsRead) {
		res := externalapi.AutomationsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.automations.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return &externalapi.AutomationsListOK{
		Items:      lo.Map(page.Items, func(a *ent.Automation, _ int) externalapi.AutomationResource { return mapper.AutomationToResource(a) }),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

// AutomationsCreate always stores a draft: activating is the send-class operation.
func (h *Handlers) AutomationsCreate(ctx context.Context, req *externalapi.CreateAutomationInput) (externalapi.AutomationsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsWrite) {
		res := externalapi.AutomationsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	in := automations.CreateInput{Name: req.Name, TriggerEvent: req.TriggerEvent, IntegrationID: convert.Int64Ptr(req.IntegrationId)}
	if req.Steps != nil {
		in.Steps = resources.AutomationSteps(req.Steps)
	}
	a, err := h.automations.Create(ctx, auth.TokenScoped(ctx), in)
	if automations.IsInvalid(err) {
		res := externalapi.AutomationsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsGet(ctx context.Context, params externalapi.AutomationsGetParams) (externalapi.AutomationsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsRead) {
		res := externalapi.AutomationsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	a, err := h.automations.Get(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsGetNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsUpdate(ctx context.Context, req *externalapi.UpdateAutomationInput, params externalapi.AutomationsUpdateParams) (externalapi.AutomationsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsWrite) {
		res := externalapi.AutomationsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)

	in := automations.UpdateInput{
		Name:         convert.StringPtr(req.Name),
		TriggerEvent: convert.StringPtr(req.TriggerEvent),
		// JSON Merge Patch: an explicit null clears the Integration, an absent key keeps it.
		IntegrationID:    convert.Int64Ptr(req.IntegrationId),
		ClearIntegration: req.IntegrationId.IsNull(),
	}
	if req.Steps != nil {
		steps := resources.AutomationSteps(req.Steps)
		in.Steps = &steps
	}
	a, err := h.automations.Update(ctx, auth.TokenScoped(ctx), id, in)
	switch {
	case errors.Is(err, automations.ErrNotFound):
		res := externalapi.AutomationsUpdateNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	case automations.IsInvalid(err):
		res := externalapi.AutomationsUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, validation.Message(err)))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsDelete(ctx context.Context, params externalapi.AutomationsDeleteParams) (externalapi.AutomationsDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsWrite) {
		res := externalapi.AutomationsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	err := h.automations.Delete(ctx, auth.TokenScoped(ctx), id)
	switch {
	case errors.Is(err, automations.ErrNotFound):
		res := externalapi.AutomationsDeleteNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	case err != nil:
		return nil, err
	}
	return &externalapi.AutomationsDeleteNoContent{}, nil
}

// AutomationsArchive archives an automation (ADR 0049); active enrollments refuse it with 409.
func (h *Handlers) AutomationsArchive(ctx context.Context, params externalapi.AutomationsArchiveParams) (externalapi.AutomationsArchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsWrite) {
		res := externalapi.AutomationsArchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	err := h.automations.Archive(ctx, ws, id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsArchiveNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	a, err := h.automations.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// AutomationsUnarchive restores an archived automation.
func (h *Handlers) AutomationsUnarchive(ctx context.Context, params externalapi.AutomationsUnarchiveParams) (externalapi.AutomationsUnarchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsWrite) {
		res := externalapi.AutomationsUnarchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	err := h.automations.Unarchive(ctx, ws, id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsUnarchiveNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	a, err := h.automations.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

// AutomationsActivate and AutomationsDeactivate need automations:activate, which
// authoring scopes never imply (ADR 0016). Over MCP, activate additionally needs
// mcp:send (the x-mcp send flag).
func (h *Handlers) AutomationsActivate(ctx context.Context, params externalapi.AutomationsActivateParams) (externalapi.AutomationsActivateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsActivate) {
		res := externalapi.AutomationsActivateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	a, err := h.automations.Activate(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsActivateNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}

func (h *Handlers) AutomationsDeactivate(ctx context.Context, params externalapi.AutomationsDeactivateParams) (externalapi.AutomationsDeactivateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeAutomationsActivate) {
		res := externalapi.AutomationsDeactivateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	a, err := h.automations.Deactivate(ctx, auth.TokenScoped(ctx), id)
	if errors.Is(err, automations.ErrNotFound) {
		res := externalapi.AutomationsDeactivateNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.AutomationToResource(a)
	return &res, nil
}
