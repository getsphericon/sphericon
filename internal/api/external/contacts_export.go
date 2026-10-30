package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/api/external/resources"
	"github.com/getsphericon/sphericon/internal/contactexport"
)

// ContactsExport streams the subject-access Data export of one contact (ADR 0021),
// needing only the contact read scope.
func (h *Handlers) ContactsExport(ctx context.Context, params externalapi.ContactsExportParams) (externalapi.ContactsExportRes, error) {
	s := auth.TokenScoped(ctx)
	var contactID *int64
	if v, ok := params.ID.Get(); ok {
		contactID = new(int64(v))
	}
	c, err := contactexport.Find(ctx, s, contactID, contactexport.Ref(params.Email.Get()))
	if err != nil {
		return nil, err
	}

	return &externalapi.ContactsExportOKApplicationOctetStreamHeaders{
		ContentDisposition: `attachment; filename="` + contactexport.Filename(c) + `"`,
		Response: externalapi.ContactsExportOKApplicationOctetStream{
			Data: contactexport.Open(ctx, s, resources.ExportMapper{Converter: mapper}, c),
		},
	}, nil
}
