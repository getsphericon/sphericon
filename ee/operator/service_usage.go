package operator

import (
	"context"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/ee/usage"
	"github.com/getsphericon/sphericon/ent"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// periodLayout is how a Usage period is written on the wire: the calendar month, YYYY-MM.
const periodLayout = "2006-01"

// ServiceUsageList is the stored figures of every Workspace for a Usage period: Live
// usage for the open one (the default), Usage snapshots for a closed one. It reads what
// the usage job stored and never recounts.
func (h *ServiceHandlers) ServiceUsageList(ctx context.Context, params serviceapi.ServiceUsageListParams) (serviceapi.ServiceUsageListRes, error) {
	period, ok := requestedPeriod(params.Period)
	if !ok {
		return &serviceapi.ServiceUsageListBadRequest{
			Status: serviceapi.NewOptInt32(400),
			Title:  serviceapi.NewOptString("Bad Request"),
			Detail: serviceapi.NewOptString(badPeriodDetail),
		}, nil
	}
	page, err := usage.List(ctx, h.module.ent, period, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]serviceapi.WorkspaceUsageResource, 0, len(page.Items))
	for _, w := range page.Items {
		items = append(items, usageResource(w))
	}
	return &serviceapi.ServiceUsageListOK{
		Items:      items,
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

const badPeriodDetail = "period must be a month written YYYY-MM"

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
		return &serviceapi.ServiceWorkspaceUsageGetBadRequest{
			Status: serviceapi.NewOptInt32(400),
			Title:  serviceapi.NewOptString("Bad Request"),
			Detail: serviceapi.NewOptString(badPeriodDetail),
		}, nil
	}
	id, ok := parseWorkspaceID(params.WorkspaceId)
	if !ok {
		p := serviceapi.ServiceWorkspaceUsageGetNotFound(workspaceNotFoundProblem())
		return &p, nil
	}
	got, err := h.module.WorkspaceUsage(ctx, id, period)
	if ent.IsNotFound(err) {
		p := serviceapi.ServiceWorkspaceUsageGetNotFound(workspaceNotFoundProblem())
		return &p, nil
	}
	if err != nil {
		return nil, err
	}
	res := usageResource(got)
	return &res, nil
}

// WorkspaceUsage reads one Workspace's stored figures for a period, scoped over the one
// Workspace the call names after its row is loaded. ent.IsNotFound for an unknown Workspace.
func (m *Module) WorkspaceUsage(ctx context.Context, workspaceID int64, period time.Time) (usage.WorkspaceUsage, error) {
	if _, err := m.ent.Workspace.Get(ctx, workspaceID); err != nil {
		return usage.WorkspaceUsage{}, err
	}
	return usage.Get(ctx, m.ent.Scoped(workspaceID), period)
}

func usageResource(w usage.WorkspaceUsage) serviceapi.WorkspaceUsageResource {
	res := serviceapi.WorkspaceUsageResource{
		WorkspaceId: serviceapi.EntityId(strconv.FormatInt(w.WorkspaceID, 10)),
		Period:      w.Period.Format(periodLayout),
		Figures:     make([]serviceapi.UsageFigure, 0, len(w.Figures)),
	}
	for _, f := range w.Figures {
		res.Figures = append(res.Figures, serviceapi.UsageFigure{
			Metric:     serviceapi.UsageMetric(f.Metric),
			Value:      f.Value,
			Final:      f.Final,
			ComputedAt: serviceapi.Timestamp(f.ComputedAt),
		})
	}
	return res
}
