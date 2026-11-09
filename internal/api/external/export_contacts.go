package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
)

// ExportContacts serves the Contacts stream (ADR 0045), the same shape as ExportEvents.
func (h *Handlers) ExportContacts(ctx context.Context, params externalapi.ExportContactsParams) (*externalapi.ExportContactsOKHeaders, error) {
	limit := int(params.Limit.Value)
	page, err := h.export.Contacts(ctx, auth.TokenScoped(ctx), params.Cursor.Or(""), limit)
	if err != nil {
		return nil, err
	}
	return &externalapi.ExportContactsOKHeaders{
		XNextCursor: page.Cursor,
		Response:    externalapi.ExportContactsOK{Data: page.Body},
	}, nil
}
