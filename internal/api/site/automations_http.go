package site

import (
	"context"
	"errors"
	"net/http"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/automations"
	"github.com/getsphericon/sphericon/internal/validation"
)

// SiteAutomationsDryRunHttpStep renders an HTTP step's body for a Contact and
// validates it; nothing is called.
func (h *Handlers) SiteAutomationsDryRunHttpStep(ctx context.Context, req *siteapi.SiteDryRunHttpStepInput, params siteapi.SiteAutomationsDryRunHttpStepParams) (siteapi.SiteAutomationsDryRunHttpStepRes, error) {
	ws, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	res, err := h.automations.DryRun(ctx, ws, int64(req.ContactId), req.Body)
	if errors.Is(err, automations.ErrContactNotFound) {
		v := siteapi.SiteAutomationsDryRunHttpStepNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	out := siteapi.SiteDryRunHttpStepResult{}
	if res.ErrorKind != "" {
		out.ErrorKind = siteapi.NewOptSiteAutomationHttpErrorKind(siteapi.SiteAutomationHttpErrorKind(res.ErrorKind))
		out.Detail = siteapi.NewOptString(res.Detail)
	} else {
		out.Body = siteapi.NewOptString(res.Body)
	}
	return &out, nil
}
