package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteAutomationsDryRunHttpStep renders an HTTP step's body for a Contact and
// validates it; nothing is called.
func (h *Handlers) SiteAutomationsDryRunHttpStep(ctx context.Context, req *siteapi.DryRunHttpStepInput, _ siteapi.SiteAutomationsDryRunHttpStepParams) (*siteapi.DryRunHttpStepResult, error) {
	ws := workspaceScope(ctx)

	res, err := h.automations.DryRun(ctx, ws, int64(req.ContactId), req.Body)
	if err != nil {
		return nil, err
	}
	out := siteapi.DryRunHttpStepResult{}
	if res.ErrorKind != "" {
		out.ErrorKind = siteapi.NewOptAutomationHttpErrorKind(siteapi.AutomationHttpErrorKind(res.ErrorKind))
		out.Detail = siteapi.NewOptString(res.Detail)
	} else {
		out.Body = siteapi.NewOptString(res.Body)
	}
	return &out, nil
}
