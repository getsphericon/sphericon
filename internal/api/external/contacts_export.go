package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// ContactsExport streams the subject-access Data export of one contact (ADR 0021),
// needing only the contact read scope.
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
