package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/exportstream"
)

// ExportErasures serves the Erasures stream (ADR 0045): NDJSON rows, the next cursor in the
// X-Next-Cursor header, behind the same gate as every Export stream.
func (h *Handlers) ExportErasures(ctx context.Context, params externalapi.ExportErasuresParams) (externalapi.ExportErasuresRes, error) {
	if !h.exportLicensed() {
		return nil, exportstream.ErrNotOffered
	}

	limit := int(params.Limit.Value)
	page, err := exportstream.Erasures(ctx, auth.TokenScoped(ctx), params.Cursor.Or(""), limit)
	if err != nil {
		return nil, err
	}
	return &externalapi.ExportErasuresOKHeaders{
		XNextCursor: page.Cursor,
		Response:    externalapi.ExportErasuresOK{Data: page.Body},
	}, nil
}
