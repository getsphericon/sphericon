package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// ExportEvents serves the Events stream (ADR 0045): NDJSON rows, the next cursor in the
// X-Next-Cursor header. A stream added later is this handler with its own typed responses
// and its own method of exportstream.Streams.
func (h *Handlers) ExportEvents(ctx context.Context, params externalapi.ExportEventsParams) (*externalapi.ExportEventsOKHeaders, error) {
	limit := int(params.Limit.Value)
	page, err := h.export.Events(ctx, auth.TokenScoped(ctx), params.Cursor.Or(""), limit)
	if err != nil {
		return nil, err
	}
	return &externalapi.ExportEventsOKHeaders{
		XNextCursor: page.Cursor,
		Response:    externalapi.ExportEventsOK{Data: page.Body},
	}, nil
}
