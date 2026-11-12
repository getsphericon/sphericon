package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) SiteRulesList(ctx context.Context, params siteapi.SiteRulesListParams) (*siteapi.SiteRulesListOK, error) {
	ws := workspaceScope(ctx)

	page, err := h.rules.List(ctx, ws, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.RulesListPage(page), nil
}

// SiteRulesCreate always stores a draft.
func (h *Handlers) SiteRulesCreate(ctx context.Context, req *siteapi.SiteCreateRuleInput, _ siteapi.SiteRulesCreateParams) (*siteapi.SiteRuleResource, error) {
	ws := workspaceScope(ctx)

	in, err := inputs.RuleCreate(req)
	if err != nil {
		return nil, err
	}
	r, err := h.rules.Create(ctx, ws, in)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) SiteRulesGet(ctx context.Context, params siteapi.SiteRulesGetParams) (*siteapi.SiteRuleResource, error) {
	ws := workspaceScope(ctx)

	r, err := h.rules.Get(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) SiteRulesUpdate(ctx context.Context, req *siteapi.SiteUpdateRuleInput, params siteapi.SiteRulesUpdateParams) (*siteapi.SiteRuleResource, error) {
	ws := workspaceScope(ctx)

	in, err := inputs.RuleUpdate(req)
	if err != nil {
		return nil, err
	}
	r, err := h.rules.Update(ctx, ws, int64(params.ID), in)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesDelete removes a Rule: one with Firings is discarded so they stay as history.
func (h *Handlers) SiteRulesDelete(ctx context.Context, params siteapi.SiteRulesDeleteParams) error {
	ws := workspaceScope(ctx)

	err := h.rules.Delete(ctx, ws, int64(params.ID))
	if err != nil {
		return err
	}
	return nil
}

// SiteRulesPublish makes the Rule active: it reacts to the Events recorded from now on.
func (h *Handlers) SiteRulesPublish(ctx context.Context, req *siteapi.RevisionInput, params siteapi.SiteRulesPublishParams) (*siteapi.SiteRuleResource, error) {
	ws := workspaceScope(ctx)

	r, err := h.rules.Publish(ctx, ws, int64(params.ID), int64(req.Revision))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesRevert drops the Rule's draft: its live version is again the latest.
func (h *Handlers) SiteRulesRevert(ctx context.Context, req *siteapi.RevisionInput, params siteapi.SiteRulesRevertParams) (*siteapi.SiteRuleResource, error) {
	ws := workspaceScope(ctx)

	r, err := h.rules.Revert(ctx, ws, int64(params.ID), int64(req.Revision))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesDeactivate makes the Rule inactive: it stops firing.
func (h *Handlers) SiteRulesDeactivate(ctx context.Context, params siteapi.SiteRulesDeactivateParams) (*siteapi.SiteRuleResource, error) {
	ws := workspaceScope(ctx)

	r, err := h.rules.Deactivate(ctx, ws, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// SiteRulesDryRun reports what a stored Event would do to the Rule, writing and running nothing.
func (h *Handlers) SiteRulesDryRun(ctx context.Context, req *siteapi.SiteDryRunRuleInput, params siteapi.SiteRulesDryRunParams) (*siteapi.SiteRuleDryRunResource, error) {
	ws := workspaceScope(ctx)

	dry, err := h.rules.DryRun(ctx, ws, int64(params.ID), int64(req.EventId))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleDryRunToResource(dry)
	return &res, nil
}

// SiteRuleFiringsList lists a Rule's Firings, newest first.
func (h *Handlers) SiteRuleFiringsList(ctx context.Context, params siteapi.SiteRuleFiringsListParams) (*siteapi.SiteRuleFiringsListOK, error) {
	ws := workspaceScope(ctx)

	page, err := h.rules.Firings(ctx, ws, int64(params.RuleId), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.RuleFiringsListPage(page), nil
}
