package site

import (
	"context"
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/api/site/resources"
	"github.com/getsphericon/sphericon/internal/contactexport"
)

// SiteContactsExport streams the subject-access Data export of one contact (ADR
// 0021). Any Workspace member may export: it needs contact read access only.
func (h *Handlers) SiteContactsExport(ctx context.Context, params siteapi.SiteContactsExportParams) (siteapi.SiteContactsExportRes, error) {
	scoped, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	var contactID *int64
	if v, ok := params.ID.Get(); ok {
		contactID = new(int64(v))
	}
	c, err := contactexport.Find(ctx, scoped, contactID, contactexport.Ref(params.Email.Get()))
	if errors.Is(err, contactexport.ErrIdentifier) {
		res := siteapi.SiteContactsExportBadRequest(problem(http.StatusBadRequest, "pass exactly one of id or email"))
		return &res, nil
	}
	if errors.Is(err, contactexport.ErrNotFound) {
		res := siteapi.SiteContactsExportNotFound(problem(http.StatusNotFound, "contact not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}

	return &siteapi.SiteContactsExportOKApplicationOctetStreamHeaders{
		ContentDisposition: `attachment; filename="` + contactexport.Filename(c) + `"`,
		Response: siteapi.SiteContactsExportOKApplicationOctetStream{
			Data: contactexport.Open(ctx, scoped, resources.ExportMapper{Converter: mapper}, c),
		},
	}, nil
}
