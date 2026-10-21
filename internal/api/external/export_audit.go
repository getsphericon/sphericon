package external

import (
	"context"
	"errors"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/exportstream"
)

// ExportAudit serves the Audit stream (ADR 0045): the Audit entries oldest first as NDJSON.
// It needs the `audit` license feature as well as `export`; without it the operation is
// absent (404), like the Audit log read is without its license.
func (h *Handlers) ExportAudit(ctx context.Context, params externalapi.ExportAuditParams) (externalapi.ExportAuditRes, error) {
	code, detail := h.exportDenied(ctx)
	if code == 0 && !h.audit.Licensed() {
		code, detail = http.StatusNotFound, "not found"
	}
	switch code {
	case http.StatusNotFound:
		res := externalapi.ExportAuditNotFound(problem(code, detail))
		return &res, nil
	case http.StatusUnauthorized:
		res := externalapi.ExportAuditUnauthorized(problem(code, detail))
		return &res, nil
	}

	limit := int(params.Limit.Value)
	page, err := exportstream.Audit(ctx, auth.TokenScoped(ctx), params.Cursor.Or(""), limit)
	if errors.Is(err, exportstream.ErrBadCursor) {
		res := externalapi.ExportAuditBadRequest(problem(http.StatusBadRequest, "invalid cursor"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.ExportAuditOKHeaders{
		XNextCursor: page.Cursor,
		Response:    externalapi.ExportAuditOK{Data: page.Body},
	}, nil
}
