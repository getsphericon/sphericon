package site

import (
	"context"
	"net/http"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/setupchecklist"
)

// SiteSetupChecklistGet maps the Workspace's Setup checklist onto the DTO.
func (h *Handlers) SiteSetupChecklistGet(ctx context.Context, params siteapi.SiteSetupChecklistGetParams) (siteapi.SiteSetupChecklistGetRes, error) {
	s, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	c, err := setupchecklist.Read(ctx, s)
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteSetupProgress{
		Steps: siteapi.SiteSetupSteps{
			Integration:   siteapi.SiteSetupStepStatus(c.Steps.Integration),
			SendingDomain: siteapi.SiteSetupStepStatus(c.Steps.SendingDomain),
			Contact:       siteapi.SiteSetupStepStatus(c.Steps.Contact),
			SentEmail:     siteapi.SiteSetupStepStatus(c.Steps.SentEmail),
		},
		Complete: c.Complete(),
		// The dismissal is stored by a later ticket; until then nothing is dismissed.
		Dismissed: false,
	}, nil
}
