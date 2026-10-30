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
	if !h.exportLicensed() {
		res := externalapi.ExportMessagesNotFound(problem(http.StatusNotFound, "not found"))
		return &res, nil
	}

	limit := int(params.Limit.Value)
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
