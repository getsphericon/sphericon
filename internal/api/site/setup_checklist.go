package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteSetupChecklistGet maps the Workspace's Setup checklist onto the DTO.
func (h *Handlers) SiteSetupChecklistGet(ctx context.Context, _ siteapi.SiteSetupChecklistGetParams) (siteapi.SiteSetupChecklistGetRes, error) {
	c, err := h.setupChecklist.Read(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	return new(mapper.SetupChecklistToResource(c)), nil
}
