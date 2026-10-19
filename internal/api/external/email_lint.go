package external

import (
	"context"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/emailrender"
)

// EmailLintLint lints a message without saving it (ADR 0040): the one lint of the save paths
// and the send planner, with the Workspace's Brand kit. What cannot render is an error;
// everything else is a warning. It needs only the right to read Templates, as it stores nothing.
func (h *Handlers) EmailLintLint(ctx context.Context, req *externalapi.LintEmailInput) (externalapi.EmailLintLintRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "templates:read") {
		res := externalapi.ProblemDetails(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	report, err := h.brandKit.Lint(ctx, auth.TokenScoped(ctx), req.Subject.Value, req.Body)
	if err != nil {
		return nil, err
	}
	res := externalapi.LintEmailResult{Errors: []externalapi.LintFinding{}, Warnings: lintFindings(report.Warnings)}
	if report.Error != nil {
		res.Errors = append(res.Errors, externalapi.LintFinding{
			Code:    externalapi.LintCode(emailrender.CodeInvalidContent),
			Message: "the message cannot be rendered",
			Detail:  externalapi.NewOptString(report.Error.Error()),
		})
	}
	return &res, nil
}

func lintFindings(list []emailrender.Finding) []externalapi.LintFinding {
	out := make([]externalapi.LintFinding, 0, len(list))
	for _, f := range list {
		out = append(out, externalapi.LintFinding{
			Code:    externalapi.LintCode(f.Code),
			Message: f.Message,
			Element: optString(f.Element),
			Detail:  optString(f.Detail),
		})
	}
	return out
}

func optString(v string) externalapi.OptString {
	if v == "" {
		return externalapi.OptString{}
	}
	return externalapi.NewOptString(v)
}
