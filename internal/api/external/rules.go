package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/condition"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/rules"
)

// Rules reuse the Automation scopes: a Rule is the light form of an Automation, so reading
// and authoring (Deactivate included) take automations:read and automations:write, and
// publishing takes the send-class automations:publish (ADR 0016, ADR 0065).

func (h *Handlers) RulesList(ctx context.Context, params externalapi.RulesListParams) (externalapi.RulesListRes, error) {
	page, err := h.rules.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	return &externalapi.RulesListOK{
		Items:      mapper.RulesToResources(page.Items),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

// RulesCreate always stores an inactive Rule: publishing is the send-class operation.
func (h *Handlers) RulesCreate(ctx context.Context, req *externalapi.CreateRuleInput) (externalapi.RulesCreateRes, error) {
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
	r, err := h.rules.Create(ctx, auth.TokenScoped(ctx), in)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) RulesGet(ctx context.Context, params externalapi.RulesGetParams) (externalapi.RulesGetRes, error) {
	r, err := h.rules.Get(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

func (h *Handlers) RulesUpdate(ctx context.Context, req *externalapi.UpdateRuleInput, params externalapi.RulesUpdateParams) (externalapi.RulesUpdateRes, error) {
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
	r, err := h.rules.Update(ctx, auth.TokenScoped(ctx), int64(params.ID), in)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesDelete purges a Rule that has no Firings; one with Firings is archived instead (409).
func (h *Handlers) RulesDelete(ctx context.Context, params externalapi.RulesDeleteParams) (externalapi.RulesDeleteRes, error) {
	err := h.rules.Delete(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	return &externalapi.RulesDeleteNoContent{}, nil
}

// RulesArchive archives a Rule (ADR 0049): it acts on nothing and keeps its Firings.
func (h *Handlers) RulesArchive(ctx context.Context, params externalapi.RulesArchiveParams) (externalapi.RulesArchiveRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	r, err := h.rules.Archive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesUnarchive restores an archived Rule, as a draft.
func (h *Handlers) RulesUnarchive(ctx context.Context, params externalapi.RulesUnarchiveParams) (externalapi.RulesUnarchiveRes, error) {
	ws := auth.TokenScoped(ctx)
	id := int64(params.ID)
	r, err := h.rules.Unarchive(ctx, ws, id)
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesPublish needs automations:publish, which authoring scopes never imply (ADR 0016, ADR 0065).
// Over MCP, it additionally needs mcp:send (the x-mcp send flag).
func (h *Handlers) RulesPublish(ctx context.Context, params externalapi.RulesPublishParams) (externalapi.RulesPublishRes, error) {
	r, err := h.rules.Publish(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesRevert drops the Rule's draft, which changes nothing the Rule does: automations:write.
func (h *Handlers) RulesRevert(ctx context.Context, params externalapi.RulesRevertParams) (externalapi.RulesRevertRes, error) {
	r, err := h.rules.Revert(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesDeactivate only stops the Rule firing, so it is authoring: automations:write.
func (h *Handlers) RulesDeactivate(ctx context.Context, params externalapi.RulesDeactivateParams) (externalapi.RulesDeactivateRes, error) {
	r, err := h.rules.Deactivate(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleToResource(r)
	return &res, nil
}

// RulesDryRun reports what a stored Event would do to the Rule. It writes and runs nothing, so
// reading scope is enough.
func (h *Handlers) RulesDryRun(ctx context.Context, req *externalapi.DryRunRuleInput, params externalapi.RulesDryRunParams) (externalapi.RulesDryRunRes, error) {
	dry, err := h.rules.DryRun(ctx, auth.TokenScoped(ctx), int64(params.ID), int64(req.EventId))
	if err != nil {
		return nil, err
	}
	res := mapper.RuleDryRunToResource(dry)
	return &res, nil
}

// RuleFiringsList lists a Rule's Firings, newest first.
func (h *Handlers) RuleFiringsList(ctx context.Context, params externalapi.RuleFiringsListParams) (externalapi.RuleFiringsListRes, error) {
	page, err := h.rules.Firings(ctx, auth.TokenScoped(ctx), int64(params.RuleId), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return &externalapi.RuleFiringsListOK{
		Items:      mapper.RuleFiringsToResources(page.Items),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}
