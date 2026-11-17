package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/api/external/resources"
	"github.com/getsphericon/sphericon/internal/domain/insights/dataexport"
	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// exports is the Data export of this surface, projected onto its contract.
var exports = dataexport.New(resources.ExportMapper{Converter: mapper})

// ContactsExport streams the subject-access Data export of one contact (ADR 0021),
// needing only the contact read scope.
func (h *Handlers) ContactsExport(ctx context.Context, params externalapi.ContactsExportParams) (externalapi.ContactsExportRes, error) {
	s := auth.TokenScoped(ctx)
	var contactID *int64
	if v, ok := params.ID.Get(); ok {
		contactID = new(int64(v))
	}
	d, err := exports.Open(ctx, s, contactID, dataexport.Ref(params.Email.Get()))
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
