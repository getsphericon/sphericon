package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/exportstream"
)

// exportLicensed is the gate every Export stream shares after the export:read scope the contract
// declares for it (ADR 0045; the scope is absent from the OAuth grantable scopes and the
// operations are hidden from MCP, ADR 0016): without the `export` license the feature is absent,
// like /staff, and a stream answers 404.
func (h *Handlers) exportLicensed() bool {
	return h.export != nil && h.export.Licensed()
}

// ExportEvents serves the Events stream (ADR 0045): NDJSON rows, the next cursor in the
// X-Next-Cursor header. A stream added later is this handler with its own typed responses
// and its own exportstream page function.
func (h *Handlers) ExportEvents(ctx context.Context, params externalapi.ExportEventsParams) (externalapi.ExportEventsRes, error) {
	if !h.exportLicensed() {
		return nil, exportstream.ErrNotOffered
	}

	limit := int(params.Limit.Value)
	page, err := exportstream.Events(ctx, auth.TokenScoped(ctx), params.Cursor.Or(""), limit)
	if err != nil {
		return nil, err
	}
	return &externalapi.ExportEventsOKHeaders{
		XNextCursor: page.Cursor,
		Response:    externalapi.ExportEventsOK{Data: page.Body},
	}, nil
}
