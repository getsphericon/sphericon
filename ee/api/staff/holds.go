package staff

import (
	"context"

	"github.com/getsphericon/sphericon/ee/domain/insights/workspaceoversight"
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
	change, err := h.module.SetHold(ctx, string(params.WorkspaceId), workspaceoversight.SetHoldInput{Credential: CurrentService(ctx), Key: params.Key, Hold: serviceInputs.HoldSet(req)})
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldSetResult{Changed: change.Changed, Hold: serviceMapper.HoldToResource(change.Hold)}, nil
}

// ServiceHoldsClear clears a hold; a missing one is cleared already.
func (h *ServiceHandlers) ServiceHoldsClear(ctx context.Context, params serviceapi.ServiceHoldsClearParams) (*serviceapi.HoldClearResult, error) {
	change, err := h.module.ClearHold(ctx, string(params.WorkspaceId), workspaceoversight.ClearHoldInput{Credential: CurrentService(ctx), Key: params.Key})
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldClearResult{Changed: change.Changed}, nil
}
