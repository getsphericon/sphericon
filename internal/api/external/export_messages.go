package external

import (
	"context"
	"errors"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/exportstream"
)

// ExportMessages serves the Outbound messages stream (ADR 0045): the same NDJSON page, cursor
// header and gates as the Events stream, over delivery metadata only.
func (h *Handlers) ExportMessages(ctx context.Context, params externalapi.ExportMessagesParams) (externalapi.ExportMessagesRes, error) {
	switch code, detail := h.exportDenied(ctx); code {
	case http.StatusNotFound:
		res := externalapi.ExportMessagesNotFound(problem(code, detail))
		return &res, nil
	case http.StatusUnauthorized:
		res := externalapi.ExportMessagesUnauthorized(problem(code, detail))
		return &res, nil
	}

	limit := exportstream.Limit(params.Limit.Get())
	page, err := exportstream.Messages(ctx, auth.TokenScoped(ctx), params.Cursor.Or(""), limit)
	if errors.Is(err, exportstream.ErrBadCursor) {
		res := externalapi.ExportMessagesBadRequest(problem(http.StatusBadRequest, "invalid cursor"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.ExportMessagesOKHeaders{
		XNextCursor: page.Cursor,
		Response:    externalapi.ExportMessagesOK{Data: page.Body},
	}, nil
}
