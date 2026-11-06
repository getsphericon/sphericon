package staff

import (
	"context"
	"time"

	"github.com/getsphericon/sphericon/ee/domain/insights/usage"
	eestaff "github.com/getsphericon/sphericon/ee/usecase/staff"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// ServiceUsageList is the stored figures of every Workspace for a Usage period: Live
// usage for the open one (the default), Usage snapshots for a closed one. It reads what
// the usage job stored and never recounts.
func (h *ServiceHandlers) ServiceUsageList(ctx context.Context, params serviceapi.ServiceUsageListParams) (*serviceapi.ServiceUsageListOK, error) {
	period, err := requestedPeriod(params.Period)
	if err != nil {
		return nil, err
	}
	page, err := h.module.UsageList(ctx, period, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return serviceMapper.UsageListPage(pagination.Map(page, func(e eestaff.UsageEntry) serviceapi.WorkspaceUsageResource {
		return usageResource(e.Usage, e.WorkspaceID)
	})), nil
}

// requestedPeriod is the Usage period a request names, zero when it names none (the module
// reads the current one). The contract's pattern lets only a month written YYYY-MM through.
func requestedPeriod(v serviceapi.OptString) (time.Time, error) {
	raw, ok := v.Get()
	if !ok {
		return time.Time{}, nil
	}
	return time.Parse(periodLayout, raw)
}

// ServiceWorkspaceUsageGet is the stored figures of one Workspace for a Usage period:
// Live usage for the open one (the default), Usage snapshots for a closed one. It reads
// what the usage job stored and never recounts.
func (h *ServiceHandlers) ServiceWorkspaceUsageGet(ctx context.Context, params serviceapi.ServiceWorkspaceUsageGetParams) (*serviceapi.WorkspaceUsageResource, error) {
	period, err := requestedPeriod(params.Period)
	if err != nil {
		return nil, err
	}
	id := string(params.WorkspaceId)
	got, err := h.module.WorkspaceUsage(ctx, id, period)
	if err != nil {
		return nil, err
	}
	return new(usageResource(got, id)), nil
}

func usageResource(w usage.WorkspaceUsage, publicID string) serviceapi.WorkspaceUsageResource {
	res := serviceMapper.UsageToResource(w)
	res.WorkspaceId = serviceapi.WorkspaceId(publicID)
	return res
}
