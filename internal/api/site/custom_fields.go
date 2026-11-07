package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/contacts/contacts"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// SiteCustomFieldsList returns the workspace's Custom field definitions — the typed,
// named attribute catalogue (ADR 0006) that feeds the segment builder. Definitions
// are auto-created on first sight at ingest. The full set is returned in a single page
// (the catalogue is small and unpaginated).
func (h *Handlers) SiteCustomFieldsList(ctx context.Context, _ siteapi.SiteCustomFieldsListParams) (*siteapi.SiteCustomFieldsListOK, error) {
	scoped := workspaceScope(ctx)

	items, err := contacts.CustomFields(ctx, scoped)
	if err != nil {
		return nil, err
	}

	return mapper.CustomFieldsListPage(pagination.Whole(items)), nil
}

// SiteCustomFieldsUpdate renames a Custom field or changes its type through the Custom field
// retyping use case (owners and admins: the operation's Permission).
func (h *Handlers) SiteCustomFieldsUpdate(ctx context.Context, req *siteapi.SiteUpdateCustomFieldInput, params siteapi.SiteCustomFieldsUpdateParams) (*siteapi.SiteCustomFieldResource, error) {
	field, err := h.customFieldRetyping.Update(ctx, workspaceScope(ctx), int64(params.ID), inputs.CustomFieldUpdate(req))
	if err != nil {
		return nil, err
	}
	res := mapper.CustomFieldToResource(field)
	return &res, nil
}
