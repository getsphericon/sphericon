package external

import (
	"context"
	"errors"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/exportstream"
)

// ExportContacts serves the Contacts stream (ADR 0045), the same shape as ExportEvents.
func (h *Handlers) ExportContacts(ctx context.Context, params externalapi.ExportContactsParams) (externalapi.ExportContactsRes, error) {
	if !h.exportLicensed() {
		res := externalapi.ExportContactsNotFound(problem(http.StatusNotFound, "not found"))
		return &res, nil
	}

	limit := int(params.Limit.Value)
	page, err := exportstream.Contacts(ctx, auth.TokenScoped(ctx), params.Cursor.Or(""), limit)
	if errors.Is(err, exportstream.ErrBadCursor) {
		res := externalapi.ExportContactsBadRequest(problem(http.StatusBadRequest, "invalid cursor"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.ExportContactsOKHeaders{
		XNextCursor: page.Cursor,
		Response:    externalapi.ExportContactsOK{Data: page.Body},
	}, nil
}
