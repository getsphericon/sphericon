package staff

import (
	"context"
	"net/http"
	"time"

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
	page, err := usage.List(ctx, h.module.ent, period, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(page.Items))
	for i, w := range page.Items {
		ids[i] = w.WorkspaceID
	}
	publicIDs, err := h.module.publicIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]serviceapi.WorkspaceUsageResource, 0, len(page.Items))
	for _, w := range page.Items {
		items = append(items, usageResource(w, publicIDs[w.WorkspaceID]))
	}
	return &serviceapi.ServiceUsageListOK{
		Items:      items,
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
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

// WorkspaceUsage reads one Workspace's stored figures for a period, scoped over the one
// Workspace the call names after its row is loaded. ent.IsNotFound for an unknown Workspace.
func (m *Module) WorkspaceUsage(ctx context.Context, workspaceID string, period time.Time) (usage.WorkspaceUsage, error) {
	s, err := m.scopeOver(ctx, workspaceID)
	if err != nil {
		return usage.WorkspaceUsage{}, err
	}
	return usage.Get(ctx, s, period)
}

func usageResource(w usage.WorkspaceUsage, publicID string) serviceapi.WorkspaceUsageResource {
	res := serviceMapper.UsageToResource(w)
	res.WorkspaceId = serviceapi.WorkspaceId(publicID)
	return res
}
