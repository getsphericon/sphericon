package external

import (
	"context"
	"errors"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/emailrender"
)

// EmailLintLint lints a message without saving it (ADR 0040): the one lint of the save paths
// and the send planner, with the Workspace's Brand kit. What cannot render is an error;
// everything else is a warning. It needs only the right to read Templates, as it stores nothing.
func (h *Handlers) EmailLintLint(ctx context.Context, req *externalapi.LintEmailInput) (externalapi.EmailLintLintRes, error) {
	subject := req.Subject.Value
	// A document is rendered as a save would render it; one that cannot be is the error finding.
	body, err := h.templates.LintBody(ctx, &subject, convert.StringPtr(req.Body), convert.DocJSON(req.Doc.Value, req.Doc.Set))
	if errors.Is(err, emailrender.ErrInvalidContent) {
		return &externalapi.LintEmailResult{
			Errors:   []externalapi.LintFinding{invalidContent(err)},
			Warnings: []externalapi.LintFinding{},
		}, nil
	}
	if err != nil {
		return nil, err
	}
	report, err := h.brandKit.Lint(ctx, auth.TokenScoped(ctx), subject, body)
	if err != nil {
		return nil, err
	}
	res := externalapi.LintEmailResult{Errors: []externalapi.LintFinding{}, Warnings: mapper.LintFindingsToResource(report.Warnings)}
	if report.Error != nil {
		res.Errors = append(res.Errors, invalidContent(report.Error))
	}
	return &res, nil
}

// invalidContent is the error finding of a message that cannot render.
func invalidContent(err error) externalapi.LintFinding {
	return mapper.LintFindingToResource(emailrender.Finding{
		Code:    emailrender.CodeInvalidContent,
		Message: "the message cannot be rendered",
		Detail:  err.Error(),
	})
}
