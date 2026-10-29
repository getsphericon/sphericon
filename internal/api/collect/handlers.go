// Package collect implements the /collect surface: tracking ingestion from customer sites,
// authenticated by the collect key.
package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"

	collectapi "github.com/getsphericon/sphericon/gen/collect"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/visitors"
	"github.com/go-faster/jx"
	"github.com/samber/lo"
)

type Handlers struct {
	bus *events.Bus
}

// NewHandlers builds the collect handlers. Payload size caps (batch, per event) are
// enforced before decoding, in internal/server.
func NewHandlers(bus *events.Bus) *Handlers {
	return &Handlers{bus: bus}
}

// rawMap decodes an ogen map[string]jx.Raw into map[string]any using
// encoding/json so that value types (float64 for numbers, nested
// map[string]any for objects, etc.) match what the service layer expects.
func rawMap(m map[string]jx.Raw) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		var decoded any
		if err := json.Unmarshal(v, &decoded); err != nil {
			return nil
		}
		out[k] = decoded
	}
	return out
}

// eventProperties is rawMap for an Event's properties, which keep numbers as the text that was
// sent (json.Number): an amount of an order Event is checked for its decimal places (ADR 0064),
// which a float64 would blur. Traits stay on rawMap, whose numbers are float64.
func eventProperties(m map[string]jx.Raw) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		dec := json.NewDecoder(bytes.NewReader(v))
		dec.UseNumber()
		var decoded any
		if err := dec.Decode(&decoded); err != nil {
			return nil
		}
		out[k] = decoded
	}
	return out
}

func (h *Handlers) CollectEventsCreate(ctx context.Context, req *collectapi.CollectEventsInput) (collectapi.CollectEventsCreateRes, error) {
	evts := lo.Map(req.Events, func(e collectapi.CollectEventInput, _ int) visitors.CollectEventInput {
		evt := visitors.CollectEventInput{
			VisitorID: e.VisitorId,
			Action:    e.Action,
		}
		if id, ok := e.ClientEventId.Get(); ok {
			evt.ClientEventID = id
		}
		if props, ok := e.Properties.Get(); ok {
			evt.Properties = eventProperties(props)
		}
		if ts, ok := e.OccurredAt.Get(); ok {
			t := time.Time(ts)
			evt.OccurredAt = &t
		}
		return evt
	})

	if err := visitors.Collect(ctx, h.bus, auth.CollectScoped(ctx), evts); err != nil {
		if rejection, ok := events.AsRejection(err); ok {
			return &collectapi.CollectEventsCreateUnprocessableEntity{
				Status: collectapi.NewOptInt32(http.StatusUnprocessableEntity),
				Title:  collectapi.NewOptString(http.StatusText(http.StatusUnprocessableEntity)),
				Detail: collectapi.NewOptString(rejection.Error()),
			}, nil
		}
		return nil, err
	}
	return &collectapi.CollectEventsCreateNoContent{}, nil
}

func (h *Handlers) CollectIdentifyCreate(ctx context.Context, req *collectapi.CollectIdentifyInput) (collectapi.CollectIdentifyCreateRes, error) {
	input := visitors.IdentifyInput{
		VisitorID: req.VisitorId,
	}
	if v, ok := req.Email.Get(); ok {
		s := string(v)
		input.Email = &s
	}
	if v, ok := req.Phone.Get(); ok {
		s := v
		input.Phone = &s
	}
	if v, ok := req.SubjectId.Get(); ok {
		s := v
		input.SubjectID = &s
	}
	if traits, ok := req.Traits.Get(); ok {
		input.Traits = rawMap(traits)
	}

	if err := visitors.Identify(ctx, h.bus, auth.CollectScoped(ctx), input); err != nil {
		return nil, err
	}
	return &collectapi.CollectOkResponse{Ok: collectapi.CollectOkResponseOkTrue}, nil
}

var _ collectapi.Handler = (*Handlers)(nil)
