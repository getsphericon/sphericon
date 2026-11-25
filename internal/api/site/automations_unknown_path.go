package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/orchestration/automations"
)

// SiteAutomationsUnknownPath answers whether a Branch with the Conditions given needs an unknown
// path, as Publish decides it (ADR 0124): the editor asks it of the version it edits, and offers
// the unknown path by the answer.
func (h *Handlers) SiteAutomationsUnknownPath(_ context.Context, req *siteapi.BranchUnknownPathInput, _ siteapi.SiteAutomationsUnknownPathParams) (*siteapi.BranchUnknownPathResult, error) {
	conditions, err := inputs.BranchConditions(req.Conditions)
	if err != nil {
		return nil, err
	}
	return &siteapi.BranchUnknownPathResult{Needed: automations.NeedsUnknownPath(conditions)}, nil
}
