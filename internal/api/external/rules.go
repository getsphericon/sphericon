package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// Rules reuse the Automation scopes: a Rule is the light form of an Automation, so reading
// and authoring (Deactivate included) take automations:read and automations:write, and
// publishing takes the send-class automations:publish (ADR 0016, ADR 0065).

func (h *Handlers) RulesList(ctx context.Context, params externalapi.RulesListParams) (*externalapi.RulesListOK, error) {
	page, err := h.rules.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.RulesListPage(page), nil
}

// RulesCreate always stores an inactive Rule: publishing is the send-class operation.
func (h *Handlers) RulesCreate(ctx context.Context, req *externalapi.CreateRuleInput) (*externalapi.RuleResource, error) {
	in, err := inputs.RuleCreate(req)
	if err != nil {
		return nil, err
	}
	r, err := h.rules.Create(ctx, auth.TokenScoped(ctx), in)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) RulesGet(ctx context.Context, params externalapi.RulesGetParams) (*externalapi.RuleResource, error) {
	r, err := h.rules.Get(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) RulesUpdate(ctx context.Context, req *externalapi.UpdateRuleInput, params externalapi.RulesUpdateParams) (*externalapi.RuleResource, error) {
	in, err := inputs.RuleUpdate(req)
	if err != nil {
		return nil, err
	}
	r, err := h.rules.Update(ctx, auth.TokenScoped(ctx), int64(params.ID), in)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesDelete removes a Rule: one with Firings is discarded so they stay as history.
func (h *Handlers) RulesDelete(ctx context.Context, params externalapi.RulesDeleteParams) error {
	err := h.rules.Delete(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return err
	}
	return nil
}

// RulesPublish needs automations:publish, which authoring scopes never imply (ADR 0016, ADR 0065).
// Over MCP, it additionally needs mcp:send (the x-mcp send flag).
func (h *Handlers) RulesPublish(ctx context.Context, params externalapi.RulesPublishParams) (*externalapi.RuleResource, error) {
	r, err := h.rules.Publish(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesRevert drops the Rule's draft, which changes nothing the Rule does: automations:write.
func (h *Handlers) RulesRevert(ctx context.Context, params externalapi.RulesRevertParams) (*externalapi.RuleResource, error) {
	r, err := h.rules.Revert(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesDeactivate only stops the Rule firing, so it is authoring: automations:write.
func (h *Handlers) RulesDeactivate(ctx context.Context, params externalapi.RulesDeactivateParams) (*externalapi.RuleResource, error) {
	r, err := h.rules.Deactivate(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesDryRun reports what a stored Event would do to the Rule. It writes and runs nothing, so
// reading scope is enough.
func (h *Handlers) RulesDryRun(ctx context.Context, req *externalapi.DryRunRuleInput, params externalapi.RulesDryRunParams) (*externalapi.RuleDryRunResource, error) {
	dry, err := h.rules.DryRun(ctx, auth.TokenScoped(ctx), int64(params.ID), int64(req.EventId))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleDryRunToResource(dry)
	return &res, nil
}

// RuleFiringsList lists a Rule's Firings, newest first.
func (h *Handlers) RuleFiringsList(ctx context.Context, params externalapi.RuleFiringsListParams) (*externalapi.RuleFiringsListOK, error) {
	page, err := h.rules.Firings(ctx, auth.TokenScoped(ctx), int64(params.RuleId), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.RuleFiringsListPage(page), nil
}
