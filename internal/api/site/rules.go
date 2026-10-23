package site

import (
	"context"
	"errors"
	"net/http"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/rules"
)

func (h *Handlers) SiteRulesList(ctx context.Context, params siteapi.SiteRulesListParams) (siteapi.SiteRulesListRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.rules.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRulesListOK{
		Items:      lo.Map(page.Items, func(r *ent.Rule, _ int) siteapi.SiteRuleResource { return mapper.RuleToResource(r) }),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

// SiteRulesCreate always stores a draft.
func (h *Handlers) SiteRulesCreate(ctx context.Context, req *siteapi.SiteCreateRuleInput, params siteapi.SiteRulesCreateParams) (siteapi.SiteRulesCreateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	in := rules.CreateInput{
		Name:          req.Name,
		TriggerEvents: req.TriggerEvents,
		Actions:       resources.RuleActions(req.Actions),
	}
	def, err := ruleCondition(req.Condition)
	if err != nil {
		return nil, err
	}
	in.Condition = def
	r, err := h.rules.Create(ctx, ws, in)
	if errors.Is(err, rules.ErrInvalid) {
		v := siteapi.SiteRulesCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) SiteRulesGet(ctx context.Context, params siteapi.SiteRulesGetParams) (siteapi.SiteRulesGetRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	r, err := h.rules.Get(ctx, ws, int64(params.ID))
	if errors.Is(err, rules.ErrNotFound) {
		v := siteapi.SiteRulesGetNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) SiteRulesUpdate(ctx context.Context, req *siteapi.SiteUpdateRuleInput, params siteapi.SiteRulesUpdateParams) (siteapi.SiteRulesUpdateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	in := rules.UpdateInput{Name: convert.StringPtr(req.Name)}
	if req.TriggerEvents != nil {
		in.TriggerEvents = &req.TriggerEvents
	}
	if req.Actions != nil {
		actions := resources.RuleActions(req.Actions)
		in.Actions = &actions
	}
	def, err := ruleCondition(req.Condition)
	if err != nil {
		return nil, err
	}
	in.Condition = def
	r, err := h.rules.Update(ctx, ws, int64(params.ID), in)
	switch {
	case errors.Is(err, rules.ErrNotFound):
		v := siteapi.SiteRulesUpdateNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	case errors.Is(err, rules.ErrInvalid):
		v := siteapi.SiteRulesUpdateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesDelete purges a Rule that has no Firings; one with Firings is archived instead (409).
func (h *Handlers) SiteRulesDelete(ctx context.Context, params siteapi.SiteRulesDeleteParams) (siteapi.SiteRulesDeleteRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	err = h.rules.Delete(ctx, ws, int64(params.ID))
	if refused, ok := inuse.As(err); ok {
		v := inUseProblem(refused)
		return &v, nil
	}
	switch {
	case errors.Is(err, rules.ErrNotFound):
		v := siteapi.SiteRulesDeleteNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	case err != nil:
		return nil, err
	}
	return &siteapi.SiteRulesDeleteNoContent{}, nil
}

// SiteRulesArchive archives a Rule (ADR 0049): it acts on nothing and keeps its Firings.
func (h *Handlers) SiteRulesArchive(ctx context.Context, params siteapi.SiteRulesArchiveParams) (siteapi.SiteRulesArchiveRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesArchiveNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.rules.Archive(ctx, ws, id)
	if errors.Is(err, rules.ErrNotFound) {
		v := siteapi.SiteRulesArchiveNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := h.rules.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesUnarchive restores an archived Rule to the list, as a draft.
func (h *Handlers) SiteRulesUnarchive(ctx context.Context, params siteapi.SiteRulesUnarchiveParams) (siteapi.SiteRulesUnarchiveRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesUnarchiveNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.rules.Unarchive(ctx, ws, id)
	if errors.Is(err, rules.ErrNotFound) {
		v := siteapi.SiteRulesUnarchiveNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	r, err := h.rules.Get(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesActivate makes the Rule react to the Events recorded from now on.
func (h *Handlers) SiteRulesActivate(ctx context.Context, params siteapi.SiteRulesActivateParams) (siteapi.SiteRulesActivateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesActivateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	r, err := h.rules.Activate(ctx, ws, int64(params.ID))
	switch {
	case errors.Is(err, rules.ErrNotFound):
		v := siteapi.SiteRulesActivateNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	case errors.Is(err, rules.ErrInvalid):
		v := siteapi.SiteRulesActivateUnprocessableEntity(problem(http.StatusUnprocessableEntity, err.Error()))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) SiteRulesDeactivate(ctx context.Context, params siteapi.SiteRulesDeactivateParams) (siteapi.SiteRulesDeactivateRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesDeactivateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	r, err := h.rules.Deactivate(ctx, ws, int64(params.ID))
	if errors.Is(err, rules.ErrNotFound) {
		v := siteapi.SiteRulesDeactivateNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesDryRun reports what a stored Event would do to the Rule, writing and running nothing.
func (h *Handlers) SiteRulesDryRun(ctx context.Context, req *siteapi.SiteDryRunRuleInput, params siteapi.SiteRulesDryRunParams) (siteapi.SiteRulesDryRunRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRulesDryRunNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	dry, err := h.rules.DryRun(ctx, ws, int64(params.ID), int64(req.EventId))
	switch {
	case errors.Is(err, rules.ErrNotFound):
		v := siteapi.SiteRulesDryRunNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	case errors.Is(err, rules.ErrEventNotFound):
		v := siteapi.SiteRulesDryRunNotFound(problem(http.StatusNotFound, "event not found"))
		return &v, nil
	case err != nil:
		return nil, err
	}
	res := mapper.RuleDryRunToResource(dry)
	return &res, nil
}

// SiteRuleFiringsList lists a Rule's Firings, newest first.
func (h *Handlers) SiteRuleFiringsList(ctx context.Context, params siteapi.SiteRuleFiringsListParams) (siteapi.SiteRuleFiringsListRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if ent.IsNotFound(err) {
		v := siteapi.SiteRuleFiringsListNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	page, err := h.rules.Firings(ctx, ws, int64(params.RuleId), pagination.ParamsOf(params.Page, params.PageSize))
	if errors.Is(err, rules.ErrNotFound) {
		v := siteapi.SiteRuleFiringsListNotFound(problem(http.StatusNotFound, "rule not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRuleFiringsListOK{
		Items: lo.Map(page.Items, func(f *ent.RuleFiring, _ int) siteapi.SiteRuleFiringResource {
			return mapper.RuleFiringToResource(f)
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

// ruleCondition is the stored form of the Condition a request carries, nil when it carries none.
func ruleCondition(opt siteapi.OptSiteConditionGroup) (*string, error) {
	group, ok := opt.Get()
	if !ok {
		return nil, nil
	}
	def, err := condition.Encode(&group)
	if err != nil {
		return nil, err
	}
	return &def, nil
}
