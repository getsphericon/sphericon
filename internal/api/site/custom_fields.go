package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/contacts"
	"github.com/getsphericon/sphericon/internal/pagination"
)

// SiteCustomFieldsList returns the workspace's Custom field definitions — the typed,
// named attribute catalogue (ADR 0006) that feeds the segment builder. Definitions
// are auto-created on first sight at ingest; this is a read-only catalogue. The full
// set is returned in a single page (the catalogue is small and unpaginated).
func (h *Handlers) SiteCustomFieldsList(ctx context.Context, _ siteapi.SiteCustomFieldsListParams) (siteapi.SiteCustomFieldsListRes, error) {
	scoped := workspaceScope(ctx)

	items, err := contacts.CustomFields(ctx, scoped)
	if err != nil {
		return nil, err
	}

	return mapper.CustomFieldsListPage(pagination.Whole(items)), nil
}
