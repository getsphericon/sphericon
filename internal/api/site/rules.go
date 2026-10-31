package site

import (
	"context"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/rules"
)

func (h *Handlers) SiteRulesList(ctx context.Context, params siteapi.SiteRulesListParams) (siteapi.SiteRulesListRes, error) {
	ws := workspaceScope(ctx)

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
func (h *Handlers) SiteRulesCreate(ctx context.Context, req *siteapi.SiteCreateRuleInput, _ siteapi.SiteRulesCreateParams) (siteapi.SiteRulesCreateRes, error) {
	ws := workspaceScope(ctx)

	in := rules.CreateInput{
		Name:          req.Name,
		TriggerEvents: req.TriggerEvents,
		Actions:       inputs.RuleActions(req.Actions),
	}
	def, err := condition.EncodeOptional(req.Condition)
	if err != nil {
		return nil, err
	}
	in.Condition = def
	r, err := h.rules.Create(ctx, ws, in)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) SiteRulesGet(ctx context.Context, params siteapi.SiteRulesGetParams) (siteapi.SiteRulesGetRes, error) {
	ws := workspaceScope(ctx)

	r, err := h.rules.Get(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) SiteRulesUpdate(ctx context.Context, req *siteapi.SiteUpdateRuleInput, params siteapi.SiteRulesUpdateParams) (siteapi.SiteRulesUpdateRes, error) {
	ws := workspaceScope(ctx)

	in := rules.UpdateInput{Name: convert.StringPtr(req.Name)}
	if req.TriggerEvents != nil {
		in.TriggerEvents = &req.TriggerEvents
	}
	if req.Actions != nil {
		actions := inputs.RuleActions(req.Actions)
		in.Actions = &actions
	}
	def, err := condition.EncodeOptional(req.Condition)
	if err != nil {
		return nil, err
	}
	in.Condition = def
	r, err := h.rules.Update(ctx, ws, int64(params.ID), in)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesDelete purges a Rule that has no Firings; one with Firings is archived instead (409).
func (h *Handlers) SiteRulesDelete(ctx context.Context, params siteapi.SiteRulesDeleteParams) (siteapi.SiteRulesDeleteRes, error) {
	ws := workspaceScope(ctx)

	err := h.rules.Delete(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteRulesDeleteNoContent{}, nil
}

// SiteRulesArchive archives a Rule (ADR 0049): it acts on nothing and keeps its Firings.
func (h *Handlers) SiteRulesArchive(ctx context.Context, params siteapi.SiteRulesArchiveParams) (siteapi.SiteRulesArchiveRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	r, err := h.rules.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesUnarchive restores an archived Rule to the list, as a draft.
func (h *Handlers) SiteRulesUnarchive(ctx context.Context, params siteapi.SiteRulesUnarchiveParams) (siteapi.SiteRulesUnarchiveRes, error) {
	ws := workspaceScope(ctx)

	id := int64(params.ID)
	r, err := h.rules.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesPublish makes the Rule active: it reacts to the Events recorded from now on.
func (h *Handlers) SiteRulesPublish(ctx context.Context, params siteapi.SiteRulesPublishParams) (siteapi.SiteRulesPublishRes, error) {
	ws := workspaceScope(ctx)

	r, err := h.rules.Publish(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesRevert drops the Rule's draft: its live version is again the latest.
func (h *Handlers) SiteRulesRevert(ctx context.Context, params siteapi.SiteRulesRevertParams) (siteapi.SiteRulesRevertRes, error) {
	ws := workspaceScope(ctx)

	r, err := h.rules.Revert(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesDeactivate makes the Rule inactive: it stops firing.
func (h *Handlers) SiteRulesDeactivate(ctx context.Context, params siteapi.SiteRulesDeactivateParams) (siteapi.SiteRulesDeactivateRes, error) {
	ws := workspaceScope(ctx)

	r, err := h.rules.Deactivate(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesDryRun reports what a stored Event would do to the Rule, writing and running nothing.
func (h *Handlers) SiteRulesDryRun(ctx context.Context, req *siteapi.SiteDryRunRuleInput, params siteapi.SiteRulesDryRunParams) (siteapi.SiteRulesDryRunRes, error) {
	ws := workspaceScope(ctx)

	dry, err := h.rules.DryRun(ctx, ws, int64(params.ID), int64(req.EventId))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleDryRunToResource(dry)
	return &res, nil
}

// SiteRuleFiringsList lists a Rule's Firings, newest first.
func (h *Handlers) SiteRuleFiringsList(ctx context.Context, params siteapi.SiteRuleFiringsListParams) (siteapi.SiteRuleFiringsListRes, error) {
	ws := workspaceScope(ctx)

	page, err := h.rules.Firings(ctx, ws, int64(params.RuleId), pagination.ParamsOf(params.Page, params.PageSize))
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
