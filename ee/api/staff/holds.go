package staff

import (
	"context"
	"errors"
	"net/http"

	eestaff "github.com/getsphericon/sphericon/ee/staff"

	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
)

// ServiceHoldsList lists a Workspace's active holds.
func (h *ServiceHandlers) ServiceHoldsList(ctx context.Context, params serviceapi.ServiceHoldsListParams) (serviceapi.ServiceHoldsListRes, error) {
	id := string(params.WorkspaceId)
	rows, err := h.module.ActiveHolds(ctx, id)
	if ent.IsNotFound(err) {
		p := serviceapi.ServiceHoldsListNotFound(workspaceNotFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldList{Items: serviceMapper.HoldsToResources(rows)}, nil
}

// ServiceHoldsSet sets or renews a hold.
func (h *ServiceHandlers) ServiceHoldsSet(ctx context.Context, req *serviceapi.HoldInput, params serviceapi.ServiceHoldsSetParams) (serviceapi.ServiceHoldsSetRes, error) {
	id := string(params.WorkspaceId)
	change, err := h.module.SetHold(ctx, CurrentService(ctx), id, params.Key, serviceInputs.HoldSet(req))
	var tooLong *eestaff.HoldTooLongError
	switch {
	case ent.IsNotFound(err):
		p := serviceapi.ServiceHoldsSetNotFound(workspaceNotFoundProblem())
		return &p, nil
	case errors.Is(err, eestaff.ErrHoldExpiryInPast), errors.Is(err, eestaff.ErrHoldActionURL), errors.As(err, &tooLong):
		p := serviceapi.ServiceHoldsSetUnprocessableEntity{
			Status: serviceapi.NewOptInt32(http.StatusUnprocessableEntity),
			Title:  serviceapi.NewOptString(http.StatusText(http.StatusUnprocessableEntity)),
			Detail: serviceapi.NewOptString(err.Error()),
		}
		return &p, nil
	case err != nil:
		return nil, err
	}
	return &serviceapi.HoldSetResult{Changed: change.Changed, Hold: serviceMapper.HoldToResource(change.Hold)}, nil
}

// ServiceHoldsClear clears a hold; a missing one is cleared already.
func (h *ServiceHandlers) ServiceHoldsClear(ctx context.Context, params serviceapi.ServiceHoldsClearParams) (serviceapi.ServiceHoldsClearRes, error) {
	id := string(params.WorkspaceId)
	change, err := h.module.ClearHold(ctx, CurrentService(ctx), id, params.Key)
	if ent.IsNotFound(err) {
		p := serviceapi.ServiceHoldsClearNotFound(workspaceNotFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	return &serviceapi.HoldClearResult{Changed: change.Changed}, nil
}

func workspaceNotFoundProblem() serviceapi.ProblemDetails {
	return serviceapi.ProblemDetails{
		Status: serviceapi.NewOptInt32(http.StatusNotFound),
		Title:  serviceapi.NewOptString(http.StatusText(http.StatusNotFound)),
		Detail: serviceapi.NewOptString("workspace not found"),
	}
}
