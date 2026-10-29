package external

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/eventlog"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
)

func (h *Handlers) EventsCreate(ctx context.Context, req *externalapi.RecordEventsInput) (externalapi.EventsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeEventsWrite) {
		res := externalapi.EventsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	inputs, err := eventInputs(req.Events, false)
	if err != nil {
		return nil, err
	}
	// The module resolves identity and publishes the batch atomically; rows land
	// asynchronously (accept-then-process).
	if err := h.eventlog.Ingest(ctx, auth.TokenScoped(ctx), inputs); err != nil {
		if rejection, ok := events.AsRejection(err); ok {
			res := externalapi.EventsCreateUnprocessableEntity(problem(http.StatusUnprocessableEntity, rejection.Error()))
			return &res, nil
		}
		return nil, err
	}
	return &externalapi.EventsCreateNoContent{}, nil
}

func eventInputs(items []externalapi.EventInput, backfill bool) ([]eventlog.Input, error) {
	inputs := make([]eventlog.Input, len(items))
	for i := range items {
		e := &items[i]
		in := eventlog.Input{
			SubjectID: e.SubjectId,
			Email:     convert.StringPtr(e.Email),
			Phone:     convert.StringPtr(e.Phone),
			Action:    e.Action,
			Backfill:  backfill,
		}
		if v, ok := e.ClientEventId.Get(); ok {
			in.ClientEventID = v
		}
		if v, ok := e.OccurredAt.Get(); ok {
			in.OccurredAt = time.Time(v)
		}
		if props, ok := e.Properties.Get(); ok {
			in.Properties = make(map[string]any, len(props))
			for key, raw := range props {
				value, err := decodeProperty(raw)
				if err != nil {
					return nil, err
				}
				in.Properties[key] = value
			}
		}
		inputs[i] = in
	}
	return inputs, nil
}

// decodeProperty reads one JSON property keeping numbers as the text that was sent: an amount of
// an order Event is checked for its decimal places (ADR 0064), which a float64 would blur.
func decodeProperty(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

// EventsBatchSubmit records each Event independently and reports a per-item result.
func (h *Handlers) EventsBatchSubmit(ctx context.Context, req *externalapi.RecordEventsBatchInput) (externalapi.EventsBatchSubmitRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeEventsWrite) {
		res := externalapi.EventsBatchSubmitUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	inputs, err := eventInputs(req.Events, req.Backfill.Or(false))
	if err != nil {
		return nil, err
	}
	errs := h.eventlog.IngestEach(ctx, auth.TokenScoped(ctx), inputs)

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
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeEventsRead) {
		res := externalapi.EventActionsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	page, err := h.eventlog.ListActions(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}

	return &externalapi.EventActionsListOK{
		Items: lo.Map(page.Items, func(a string, _ int) externalapi.EventActionResource {
			return externalapi.EventActionResource{Action: a}
		}),
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}
