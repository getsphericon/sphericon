package staff

import (
	"context"
	"net/http"
	"time"

	eestaff "github.com/getsphericon/sphericon/ee/staff"

	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// ServiceUsageList is the stored figures of every Workspace for a Usage period: Live
// usage for the open one (the default), Usage snapshots for a closed one. It reads what
// the usage job stored and never recounts.
func (h *ServiceHandlers) ServiceUsageList(ctx context.Context, params serviceapi.ServiceUsageListParams) (serviceapi.ServiceUsageListRes, error) {
	period, ok := requestedPeriod(params.Period)
	if !ok {
		p := serviceapi.ServiceUsageListBadRequest(badPeriodProblem())
		return &p, nil
	}
	page, err := h.module.UsageList(ctx, period, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return serviceMapper.UsageListPage(pagination.Map(page, func(e eestaff.UsageEntry) serviceapi.WorkspaceUsageResource {
		return usageResource(e.Usage, e.WorkspaceID)
	})), nil
}

// badPeriodProblem is the 400 answered for a period that is not a month written YYYY-MM.
func badPeriodProblem() serviceapi.ProblemDetails {
	return serviceapi.ProblemDetails{
		Status: serviceapi.NewOptInt32(http.StatusBadRequest),
		Title:  serviceapi.NewOptString(http.StatusText(http.StatusBadRequest)),
		Detail: serviceapi.NewOptString("period must be a month written YYYY-MM"),
	}
}

// requestedPeriod is the Usage period a request names, the current one when absent;
// false when it is not a month written YYYY-MM.
func requestedPeriod(v serviceapi.OptString) (time.Time, bool) {
	raw, ok := v.Get()
	if !ok {
		return usage.PeriodOf(time.Now()), true
	}
	parsed, err := time.Parse(periodLayout, raw)
	return parsed, err == nil
}

// ServiceWorkspaceUsageGet is the stored figures of one Workspace for a Usage period:
// Live usage for the open one (the default), Usage snapshots for a closed one. It reads
// what the usage job stored and never recounts.
func (h *ServiceHandlers) ServiceWorkspaceUsageGet(ctx context.Context, params serviceapi.ServiceWorkspaceUsageGetParams) (serviceapi.ServiceWorkspaceUsageGetRes, error) {
	period, ok := requestedPeriod(params.Period)
	if !ok {
		p := serviceapi.ServiceWorkspaceUsageGetBadRequest(badPeriodProblem())
		return &p, nil
	}
	id := string(params.WorkspaceId)
	got, err := h.module.WorkspaceUsage(ctx, id, period)
	if ent.IsNotFound(err) {
		p := serviceapi.ServiceWorkspaceUsageGetNotFound(workspaceNotFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	res := usageResource(got, id)
	return &res, nil
}

func usageResource(w usage.WorkspaceUsage, publicID string) serviceapi.WorkspaceUsageResource {
	res := serviceMapper.UsageToResource(w)
	res.WorkspaceId = serviceapi.WorkspaceId(publicID)
	return res
}
