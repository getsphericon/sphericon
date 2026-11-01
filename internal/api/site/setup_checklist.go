package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/setupchecklist"
)

// SiteSetupChecklistGet maps the Workspace's Setup checklist onto the DTO.
func (h *Handlers) SiteSetupChecklistGet(ctx context.Context, _ siteapi.SiteSetupChecklistGetParams) (siteapi.SiteSetupChecklistGetRes, error) {
	c, err := setupchecklist.Read(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	return new(mapper.SetupChecklistToResource(c)), nil
}
