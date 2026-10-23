package external

import (
	"context"
	"errors"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/exportstream"
)

// externalapi.ApiTokenScopeExportRead gates every Export stream (ADR 0045). It is deliberately absent from the
// OAuth grantable scopes, and the operations are hidden from MCP (ADR 0016).
// exportDenied is the gate every Export stream shares: the `export` license first, so an
// unlicensed instance answers 404 whatever the token is (the feature is absent, like
// /staff), then the export:read scope (401, like the other scopes). It returns the status
// of the refusal and its detail, or 0 when the request may proceed.
func (h *Handlers) exportDenied(ctx context.Context) (status int, detail string) {
	if h.export == nil || !h.export.Licensed() {
		return http.StatusNotFound, "not found"
	}
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeExportRead) {
		return http.StatusUnauthorized, "insufficient scope"
	}
	return 0, ""
}

// ExportEvents serves the Events stream (ADR 0045): NDJSON rows, the next cursor in the
// X-Next-Cursor header. A stream added later is this handler with its own typed responses
// and its own exportstream page function.
func (h *Handlers) ExportEvents(ctx context.Context, params externalapi.ExportEventsParams) (externalapi.ExportEventsRes, error) {
	switch code, detail := h.exportDenied(ctx); code {
	case http.StatusNotFound:
		res := externalapi.ExportEventsNotFound(problem(code, detail))
		return &res, nil
	case http.StatusUnauthorized:
		res := externalapi.ExportEventsUnauthorized(problem(code, detail))
		return &res, nil
	}

	limit := int(params.Limit.Value)
	page, err := exportstream.Events(ctx, auth.TokenScoped(ctx), params.Cursor.Or(""), limit)
	if errors.Is(err, exportstream.ErrBadCursor) {
		res := externalapi.ExportEventsBadRequest(problem(http.StatusBadRequest, "invalid cursor"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.ExportEventsOKHeaders{
		XNextCursor: page.Cursor,
		Response:    externalapi.ExportEventsOK{Data: page.Body},
	}, nil
}
