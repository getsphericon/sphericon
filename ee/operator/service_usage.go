package operator

import (
	"context"
	"strconv"
	"time"

	"github.com/getsphericon/sphericon/ee/usage"
	serviceapi "github.com/getsphericon/sphericon/gen/service"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// periodLayout is how a Usage period is written on the wire: the calendar month, YYYY-MM.
const periodLayout = "2006-01"

// ServiceUsageList is the stored figures of every Workspace for a Usage period: Live
// usage for the open one (the default), Usage snapshots for a closed one. It reads what
// the usage job stored and never recounts.
func (h *ServiceHandlers) ServiceUsageList(ctx context.Context, params serviceapi.ServiceUsageListParams) (serviceapi.ServiceUsageListRes, error) {
	period := usage.PeriodOf(time.Now())
	if v, ok := params.Period.Get(); ok {
		parsed, err := time.Parse(periodLayout, v)
		if err != nil {
			bad := serviceapi.ServiceUsageListBadRequest{
				Status: serviceapi.NewOptInt32(400),
				Title:  serviceapi.NewOptString("Bad Request"),
				Detail: serviceapi.NewOptString("period must be a month written YYYY-MM"),
			}
			return &bad, nil
		}
		period = parsed
	}
	page, err := usage.List(ctx, h.module.ent, period, pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]serviceapi.WorkspaceUsageResource, 0, len(page.Items))
	for _, w := range page.Items {
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
		items = append(items, res)
	}
	return &serviceapi.ServiceUsageListOK{
		Items:      items,
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}
