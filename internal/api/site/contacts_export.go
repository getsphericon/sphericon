package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// SiteContactsExport streams the subject-access Data export of one contact (ADR
// 0021). Any Workspace member may export: it needs contact read access only.
func (h *Handlers) SiteContactsExport(ctx context.Context, params siteapi.SiteContactsExportParams) (siteapi.SiteContactsExportRes, error) {
	scoped := workspaceScope(ctx)

	d, err := h.exports.Open(ctx, scoped, inputs.ContactExport(params))
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
