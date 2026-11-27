package site

import (
	"context"
	"net/url"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/kernel/paths"
	"github.com/getsphericon/sphericon/internal/kernel/publicid"
	"github.com/getsphericon/sphericon/internal/kernel/seqreader"
)

// SiteContactImportsUpload keeps an uploaded CSV file of Contacts as a new import (ADR 0138), the
// caller its Member. The body's own cap (SPHERICON_CSV_IMPORT_MAX_BYTES) is the server's.
func (h *Handlers) SiteContactImportsUpload(ctx context.Context, req siteapi.SiteContactImportsUploadReq, _ siteapi.SiteContactImportsUploadParams) (*siteapi.SiteContactImport, error) {
	in := inputs.ContactImportUpload(req)
	in.MembershipID = callerMembership(ctx).ID
	v, err := h.contactImports.Upload(ctx, workspaceScope(ctx), in)
	if err != nil {
		return nil, err
	}
	res := mapper.ContactImportToResource(v)
	return &res, nil
}

// SiteContactImportsGet reads an import.
func (h *Handlers) SiteContactImportsGet(ctx context.Context, params siteapi.SiteContactImportsGetParams) (*siteapi.SiteContactImport, error) {
	v, err := h.contactImports.Get(ctx, workspaceScope(ctx), int64(params.ImportId))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactImportToResource(v)
	return &res, nil
}

// SiteContactImportsSaveMapping saves how an import reads its file.
func (h *Handlers) SiteContactImportsSaveMapping(ctx context.Context, req *siteapi.ContactImportMappingInput, params siteapi.SiteContactImportsSaveMappingParams) (*siteapi.SiteContactImport, error) {
	v, err := h.contactImports.SaveMapping(ctx, workspaceScope(ctx), int64(params.ImportId), inputs.ContactImportMapping(req))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactImportToResource(v)
	return &res, nil
}

// SiteContactImportsPreview previews an import, writing nothing.
func (h *Handlers) SiteContactImportsPreview(ctx context.Context, params siteapi.SiteContactImportsPreviewParams) (*siteapi.SiteContactImportPreview, error) {
	p, err := h.contactImports.Preview(ctx, workspaceScope(ctx), int64(params.ImportId))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactImportPreviewToResource(p)
	return &res, nil
}

// SiteContactImportsRun runs an import: its job writes the rows.
func (h *Handlers) SiteContactImportsRun(ctx context.Context, params siteapi.SiteContactImportsRunParams) (*siteapi.SiteContactImport, error) {
	v, err := h.contactImports.Run(ctx, workspaceScope(ctx), int64(params.ImportId))
	if err != nil {
		return nil, err
	}
	res := mapper.ContactImportToResource(v)
	return &res, nil
}

// SiteContactImportsReportLink issues the link to an import's refused rows' report, bound to the
// caller's session (downloadlinks): the path of SiteContactImportsReport with the link's token.
func (h *Handlers) SiteContactImportsReportLink(ctx context.Context, params siteapi.SiteContactImportsReportLinkParams) (*siteapi.DownloadLink, error) {
	id := int64(params.ImportId)
	link, err := h.contactImports.ReportLink(ctx, workspaceScope(ctx), id, auth.GetSiteAuth(ctx).Token)
	if err != nil {
		return nil, err
	}
	path := paths.Site + "/workspaces/" + url.PathEscape(string(params.WorkspaceId)) + "/contact-imports/" + publicid.Encode(id) + "/report/" + url.PathEscape(link.Token)
	res := mapper.DownloadLinkToResource(resources.DownloadLinkView{URL: path, ExpiresAt: link.ExpiresAt})
	return &res, nil
}

// SiteContactImportsReport streams an import's refused rows' report, once, to the session its
// link was issued in.
func (h *Handlers) SiteContactImportsReport(ctx context.Context, params siteapi.SiteContactImportsReportParams) (*siteapi.SiteContactImportsReportOKHeaders, error) {
	in := inputs.ContactImportReport(params)
	in.Session = auth.GetSiteAuth(ctx).Token
	d, err := h.contactImports.Report(ctx, workspaceScope(ctx), in)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteContactImportsReportOKHeaders{
		ContentDisposition: `attachment; filename="` + d.Filename + `"`,
		Response:           siteapi.SiteContactImportsReportOK{Data: seqreader.New(d.Chunks)},
	}, nil
}
