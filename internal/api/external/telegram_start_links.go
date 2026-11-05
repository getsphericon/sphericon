package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/kernel/convert"
)

// TelegramStartLinksIssue issues the Contact's start link on the named bot or the default one.
func (h *Handlers) TelegramStartLinksIssue(ctx context.Context, req *externalapi.IssueTelegramStartLinkInput, params externalapi.TelegramStartLinksIssueParams) (*externalapi.TelegramStartLinkResource, error) {
	link, err := h.bindings.IssueStartLink(ctx, auth.TokenScoped(ctx), int64(params.ContactId), convert.Int64Ptr(req.IntegrationId))
	if err != nil {
		return nil, err
	}
	resource := mapper.StartLinkToResource(link)
	return &resource, nil
}
