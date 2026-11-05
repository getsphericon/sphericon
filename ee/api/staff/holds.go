package staff

import (
	"context"

	serviceapi "github.com/getsphericon/sphericon/gen/service"
)

// ServiceHoldsList lists a Workspace's active holds.
func (h *ServiceHandlers) ServiceHoldsList(ctx context.Context, params serviceapi.ServiceHoldsListParams) (serviceapi.ServiceHoldsListRes, error) {
	rows, err := h.module.ActiveHolds(ctx, string(params.WorkspaceId))
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldList{Items: serviceMapper.HoldsToResources(rows)}, nil
}

// ServiceHoldsSet sets or renews a hold.
func (h *ServiceHandlers) ServiceHoldsSet(ctx context.Context, req *serviceapi.HoldInput, params serviceapi.ServiceHoldsSetParams) (serviceapi.ServiceHoldsSetRes, error) {
	change, err := h.module.SetHold(ctx, CurrentService(ctx), string(params.WorkspaceId), params.Key, serviceInputs.HoldSet(req))
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldSetResult{Changed: change.Changed, Hold: serviceMapper.HoldToResource(change.Hold)}, nil
}

// ServiceHoldsClear clears a hold; a missing one is cleared already.
func (h *ServiceHandlers) ServiceHoldsClear(ctx context.Context, params serviceapi.ServiceHoldsClearParams) (serviceapi.ServiceHoldsClearRes, error) {
	change, err := h.module.ClearHold(ctx, CurrentService(ctx), string(params.WorkspaceId), params.Key)
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldClearResult{Changed: change.Changed}, nil
}
