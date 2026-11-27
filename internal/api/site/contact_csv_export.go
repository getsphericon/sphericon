package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// SiteContactCsvExportsCreate asks for the CSV export of a Segment's Contacts, or of all of them
// (ADR 0138), and answers with its one-time link, bound to the request's session.
func (h *Handlers) SiteContactCsvExportsCreate(ctx context.Context, req *siteapi.SiteContactCsvExportInput, _ siteapi.SiteContactCsvExportsCreateParams) (*siteapi.SiteContactCsvExportLink, error) {
	in := inputs.ContactCsvExport(req)
	in.Session = auth.GetSiteAuth(ctx).Token
	link, err := h.contactsExport.Link(ctx, workspaceScope(ctx), in)
	if err != nil {
		return nil, err
	}
	return mapper.ContactCsvExportLink(link), nil
}

// SiteContactCsvExportsDownload follows the link of a CSV export of Contacts and streams the file
// as it is read (ADR 0138): the export is audited before the first row leaves.
func (h *Handlers) SiteContactCsvExportsDownload(ctx context.Context, params siteapi.SiteContactCsvExportsDownloadParams) (*siteapi.SiteContactCsvExportsDownloadOKHeaders, error) {
	in := inputs.ContactCsvExportFollow(params)
	in.Session = auth.GetSiteAuth(ctx).Token
	d, err := h.contactsExport.Open(ctx, workspaceScope(ctx), in)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactCsvExportsDownloadOKHeaders{
		ContentDisposition: `attachment; filename="` + d.Filename + `"`,
		Response:           siteapi.SiteContactCsvExportsDownloadOK{Data: seqreader.New(d.Chunks)},
	}, nil
}
