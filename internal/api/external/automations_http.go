package external

import (
	"context"
	"errors"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/automations"
)

// AutomationsDryRunHttpStep renders an HTTP step's body for a Contact and validates
// it; nothing is called. It reads a Contact, so it needs both scopes.
func (h *Handlers) AutomationsDryRunHttpStep(ctx context.Context, req *externalapi.DryRunHttpStepInput) (externalapi.AutomationsDryRunHttpStepRes, error) {
	tok := auth.GetTokenAuth(ctx)
	if !auth.HasScope(tok, scopeAutomationsWrite) || !auth.HasScope(tok, "contacts:read") {
		res := externalapi.AutomationsDryRunHttpStepUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}

	res, err := h.automations.DryRun(ctx, auth.TokenScoped(ctx), int64(req.ContactId), req.Body)
	if errors.Is(err, automations.ErrContactNotFound) {
		v := externalapi.AutomationsDryRunHttpStepNotFound(problem(http.StatusNotFound, "contact not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	out := externalapi.DryRunHttpStepResult{}
	if res.ErrorKind != "" {
		out.ErrorKind = externalapi.NewOptAutomationHttpErrorKind(externalapi.AutomationHttpErrorKind(res.ErrorKind))
		out.Detail = externalapi.NewOptString(res.Detail)
	} else {
		out.Body = externalapi.NewOptString(res.Body)
	}
	return &out, nil
}
