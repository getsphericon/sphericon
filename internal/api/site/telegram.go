package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/convert"
)

// SiteTelegramListBindings lists the Telegram chats a Contact is reached through.
func (h *Handlers) SiteTelegramListBindings(ctx context.Context, params siteapi.SiteTelegramListBindingsParams) ([]siteapi.SiteBindingResource, error) {
	chats, err := h.bindings.ForContact(ctx, workspaceScope(ctx), int64(params.ContactId))
	if err != nil {
		return nil, err
	}
	return mapper.BindingsToResources(chats), nil
}

// SiteTelegramIssueStartLink issues the Contact's start link on the named bot or the default one.
func (h *Handlers) SiteTelegramIssueStartLink(ctx context.Context, req *siteapi.IssueTelegramStartLinkInput, params siteapi.SiteTelegramIssueStartLinkParams) (*siteapi.TelegramStartLinkResource, error) {
	link, err := h.bindings.IssueStartLink(ctx, workspaceScope(ctx), int64(params.ContactId), convert.Int64Ptr(req.IntegrationId))
	if err != nil {
		return nil, err
	}
	resource := mapper.StartLinkToResource(link)
	return &resource, nil
}
