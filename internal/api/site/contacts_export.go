package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/domain/insights/dataexport"
	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// SiteContactsExport streams the subject-access Data export of one contact (ADR
// 0021). Any Workspace member may export: it needs contact read access only.
func (h *Handlers) SiteContactsExport(ctx context.Context, params siteapi.SiteContactsExportParams) (siteapi.SiteContactsExportRes, error) {
	scoped := workspaceScope(ctx)

	var contactID *int64
	if v, ok := params.ID.Get(); ok {
		contactID = new(int64(v))
	}
	d, err := dataexport.Open(ctx, scoped, resources.ExportMapper{Converter: mapper}, contactID, dataexport.Ref(params.Email.Get()))
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteContactsExportOKApplicationOctetStreamHeaders{
		ContentDisposition: `attachment; filename="` + d.Filename + `"`,
		Response: siteapi.SiteContactsExportOKApplicationOctetStream{
			Data: seqreader.New(d.Chunks),
		},
	}, nil
}
