package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// ContactsExport streams the subject-access Data export of one contact (ADR 0021). It needs
// the contacts:export scope, whose User holds the Permission of the same name, and the export is
// audited as contact.export (ADR 0022). The CSV export of many Contacts is /site's only (ADR 0138).
func (h *Handlers) ContactsExport(ctx context.Context, params externalapi.ContactsExportParams) (externalapi.ContactsExportRes, error) {
	s := auth.TokenScoped(ctx)
	d, err := h.exports.Open(ctx, s, inputs.ContactExport(params))
	if err != nil {
		return nil, err
	}

	return &externalapi.ContactsExportOKApplicationOctetStreamHeaders{
		ContentDisposition: `attachment; filename="` + d.Filename + `"`,
		Response: externalapi.ContactsExportOKApplicationOctetStream{
			Data: seqreader.New(d.Chunks),
		},
	}, nil
}
