package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/conditionspike"
)

// SPIKE (#1674): store a Condition of each place and answer it as read back.

func (h *Handlers) SiteConditionSpikeSegment(ctx context.Context, req *siteapi.SpikeSegmentConditionInput, _ siteapi.SiteConditionSpikeSegmentParams) (*siteapi.SpikeSegmentConditionResult, error) {
	in, err := inputs.SpikeSegmentCondition(req)
	if err != nil {
		return nil, err
	}
	row, err := conditionspike.Store(ctx, workspaceScope(ctx), conditionspike.PlaceSegment, in)
	if err != nil {
		return nil, err
	}
	res := mapper.SpikeSegmentConditionResult(row)
	return &res, nil
}

func (h *Handlers) SiteConditionSpikeRule(ctx context.Context, req *siteapi.SpikeRuleConditionInput, _ siteapi.SiteConditionSpikeRuleParams) (*siteapi.SpikeRuleConditionResult, error) {
	in, err := inputs.SpikeRuleCondition(req)
	if err != nil {
		return nil, err
	}
	row, err := conditionspike.Store(ctx, workspaceScope(ctx), conditionspike.PlaceRule, in)
	if err != nil {
		return nil, err
	}
	res := mapper.SpikeRuleConditionResult(row)
	return &res, nil
}
