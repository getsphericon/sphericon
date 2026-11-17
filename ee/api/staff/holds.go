package staff

import (
	"context"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
)

// ServiceHoldsList lists a Workspace's active holds.
func (h *ServiceHandlers) ServiceHoldsList(ctx context.Context, params serviceapi.ServiceHoldsListParams) (*serviceapi.HoldList, error) {
	rows, err := h.module.ActiveHolds(ctx, string(params.WorkspaceId))
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldList{Items: serviceMapper.HoldsToResources(rows)}, nil
}

// ServiceHoldsSet sets or renews a hold.
func (h *ServiceHandlers) ServiceHoldsSet(ctx context.Context, req *serviceapi.HoldInput, params serviceapi.ServiceHoldsSetParams) (*serviceapi.HoldSetResult, error) {
	in := serviceInputs.SetHold(req)
	in.Credential, in.Key = CurrentService(ctx), params.Key
	change, err := h.module.SetHold(ctx, string(params.WorkspaceId), in)
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldSetResult{Changed: change.Changed, Hold: serviceMapper.HoldToResource(change.Hold)}, nil
}

// ServiceHoldsClear clears a hold; a missing one is cleared already.
func (h *ServiceHandlers) ServiceHoldsClear(ctx context.Context, params serviceapi.ServiceHoldsClearParams) (*serviceapi.HoldClearResult, error) {
	in := serviceInputs.ClearHold(params)
	in.Credential = CurrentService(ctx)
	change, err := h.module.ClearHold(ctx, string(params.WorkspaceId), in)
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldClearResult{Changed: change.Changed}, nil
}
