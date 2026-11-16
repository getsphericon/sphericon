package site

import (
	"context"

	siteapi "github.com/getsphericon/sphericon/gen/site"
)

// SiteIdentitySecretGet returns the Workspace's Identity secret, none until one is generated
// (ADR 0171).
func (h *Handlers) SiteIdentitySecretGet(ctx context.Context, _ siteapi.SiteIdentitySecretGetParams) (*siteapi.SiteIdentitySecretResource, error) {
	secret, err := h.identityTokens.Secret(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	return identitySecretResource(secret), nil
}

// SiteIdentitySecretRegenerate generates the Workspace's Identity secret, replacing the one it had,
// and returns it.
func (h *Handlers) SiteIdentitySecretRegenerate(ctx context.Context, _ siteapi.SiteIdentitySecretRegenerateParams) (*siteapi.SiteIdentitySecretResource, error) {
	secret, err := h.identityTokens.Regenerate(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	return identitySecretResource(secret), nil
}

func identitySecretResource(secret string) *siteapi.SiteIdentitySecretResource {
	res := &siteapi.SiteIdentitySecretResource{}
	if secret != "" {
		res.Secret = siteapi.NewOptString(secret)
	}
	return res
}
