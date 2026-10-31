package external

import (
	"context"

	"github.com/samber/lo"

	externalapi "github.com/getsphericon/sphericon/gen/external"

	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) EventsCreate(ctx context.Context, req *externalapi.RecordEventsInput) (externalapi.EventsCreateRes, error) {
	batch, err := inputs.Events(req.Events, false)
	if err != nil {
		return nil, err
	}
	// The module resolves identity and publishes the batch atomically; rows land
	// asynchronously (accept-then-process).
	if err := h.eventlog.Ingest(ctx, auth.TokenScoped(ctx), batch); err != nil {
		return nil, err
	}
	return &externalapi.EventsCreateNoContent{}, nil
}

// EventsBatchSubmit records each Event independently and reports a per-item result.
func (h *Handlers) EventsBatchSubmit(ctx context.Context, req *externalapi.RecordEventsBatchInput) (externalapi.EventsBatchSubmitRes, error) {
	batch, err := inputs.Events(req.Events, req.Backfill.Or(false))
	if err != nil {
		return nil, err
	}
	errs := h.eventlog.IngestEach(ctx, auth.TokenScoped(ctx), batch)

	results := make([]externalapi.EventBatchItemResult, len(errs))
	for i, e := range errs {
		results[i] = externalapi.EventBatchItemResult{Index: int32(i), Status: externalapi.EventBatchStatusAccepted}
		if e != nil {
			results[i].Status = externalapi.EventBatchStatusFailed
			results[i].Error = externalapi.NewOptString(itemError(e))
		}
	}
	return &externalapi.RecordEventsBatchResult{Results: results}, nil
}

func (h *Handlers) EventActionsList(ctx context.Context, params externalapi.EventActionsListParams) (externalapi.EventActionsListRes, error) {
	page, err := h.eventlog.ListActions(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	out := mapper.EventActionsListPage(pagination.WithItems(page, lo.Map(page.Items, func(a string, _ int) externalapi.EventActionResource {
		return externalapi.EventActionResource{Action: a}
	})))
	return &out, nil
}
