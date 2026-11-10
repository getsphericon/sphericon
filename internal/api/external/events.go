package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"

	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

func (h *Handlers) EventsCreate(ctx context.Context, req *externalapi.RecordEventsInput) error {
	batch, err := inputs.Events(req.Events, false)
	if err != nil {
		return err
	}
	// The module resolves identity and publishes the batch atomically; rows land
	// asynchronously (accept-then-process).
	return h.eventrecording.Record(ctx, auth.TokenScoped(ctx), batch)
}

// EventsBatchSubmit records each Event independently and reports a per-item result.
func (h *Handlers) EventsBatchSubmit(ctx context.Context, req *externalapi.RecordEventsBatchInput) (*externalapi.RecordEventsBatchResult, error) {
	batch, err := inputs.Events(req.Events, req.Backfill.Or(false))
	if err != nil {
		return nil, err
	}
	outcomes := h.eventrecording.RecordEach(ctx, auth.TokenScoped(ctx), batch).Items

	results := make([]externalapi.EventBatchItemResult, len(outcomes))
	for i, o := range outcomes {
		results[i] = externalapi.EventBatchItemResult{Index: int32(i), Status: externalapi.EventBatchStatusAccepted}
		if o.Err != nil {
			results[i].Status = externalapi.EventBatchStatusFailed
			results[i].Error = externalapi.NewOptString(itemError(o.Err))
		}
	}
	return &externalapi.RecordEventsBatchResult{Results: results}, nil
}

func (h *Handlers) EventActionsList(ctx context.Context, params externalapi.EventActionsListParams) (*externalapi.EventActionsListOK, error) {
	// The distinct actions are a short catalogue, read whole as the site's list reads them.
	all, err := events.DistinctActions(ctx, auth.TokenScoped(ctx))
	if err != nil {
		return nil, err
	}
	page := pagination.Paginate(all, convert.Ptr(params.Page), convert.Ptr(params.PageSize))
	return mapper.EventActionsListPage(page), nil
}
