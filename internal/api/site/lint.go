package site

import (
	"context"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// lintWarnings is the lint of ADR 0040 over the content a save has just stored, against the
// Workspace's Brand kit. An HTML body written by the request is linted as written, before the
// sanitizer, so a construct the save removed is still named; otherwise the stored body is linted
// (a document's rendered HTML, or a write of the subject alone). Warnings never block: a
// message that cannot render was already refused by the save.
func (h *Handlers) lintWarnings(ctx context.Context, s *ent.Scoped, subject string, written siteapi.OptString, stored string) ([]siteapi.LintFinding, error) {
	body := stored
	if written.Set {
		body = written.Value
	}
	report, err := h.brandKit.Lint(ctx, s, subject, body)
	if err != nil {
		return nil, err
	}
	return mapper.LintFindingsToResource(report.Warnings), nil
}
