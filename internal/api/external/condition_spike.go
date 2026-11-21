package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/contacts/conditionspike"
)

// SPIKE (#1674): store a Condition of each place and answer it as read back.

func (h *Handlers) ConditionSpikeSegment(ctx context.Context, req *externalapi.SpikeSegmentConditionInput) (*externalapi.SpikeSegmentConditionResult, error) {
	in, err := inputs.SpikeSegmentCondition(req)
	if err != nil {
		return nil, err
	}
	row, err := conditionspike.Store(ctx, auth.TokenScoped(ctx), conditionspike.PlaceSegment, in)
	if err != nil {
		return nil, err
	}
	res := mapper.SpikeSegmentConditionResult(row)
	return &res, nil
}

func (h *Handlers) ConditionSpikeRule(ctx context.Context, req *externalapi.SpikeRuleConditionInput) (*externalapi.SpikeRuleConditionResult, error) {
	in, err := inputs.SpikeRuleCondition(req)
	if err != nil {
		return nil, err
	}
	row, err := conditionspike.Store(ctx, auth.TokenScoped(ctx), conditionspike.PlaceRule, in)
	if err != nil {
		return nil, err
	}
	res := mapper.SpikeRuleConditionResult(row)
	return &res, nil
}
