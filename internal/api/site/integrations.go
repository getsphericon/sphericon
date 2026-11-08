package site

import (
	"context"

	"github.com/samber/lo"

	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
)

// SiteIntegrationsList returns the workspace's sending-provider integrations
// with secrets redacted.
func (h *Handlers) SiteIntegrationsList(ctx context.Context, _ siteapi.SiteIntegrationsListParams) ([]siteapi.SiteIntegrationResource, error) {
	views, err := h.integrations.All(ctx, workspaceScope(ctx))
	if err != nil {
		return nil, err
	}
	return lo.Map(views, func(v integrations.View, _ int) siteapi.SiteIntegrationResource { return integrationResource(v) }), nil
}

// SiteIntegrationsCreate stores a new provider integration; credentials are
// encrypted at rest and never returned. Owners and admins only.
func (h *Handlers) SiteIntegrationsCreate(ctx context.Context, req *siteapi.CreateIntegrationInput, _ siteapi.SiteIntegrationsCreateParams) (*siteapi.SiteIntegrationResource, error) {
	in, err := inputs.IntegrationCreate(req)
	if err != nil {
		return nil, err
	}
	v, err := h.integrations.Create(ctx, workspaceScope(ctx), in)
	if err != nil {
		return nil, err
	}
	res := integrationResource(v)
	return &res, nil
}

// SiteIntegrationsGet returns one integration with secrets redacted.
func (h *Handlers) SiteIntegrationsGet(ctx context.Context, params siteapi.SiteIntegrationsGetParams) (*siteapi.SiteIntegrationResource, error) {
	v, err := h.integrations.View(ctx, workspaceScope(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := integrationResource(v)
	return &res, nil
}

// SiteIntegrationsUpdate edits an integration. Omitting config keeps the stored
// credentials; a supplied config must match the integration's provider kind.
// Within a supplied config, blank secret fields (password, secret access key)
// keep the stored secret rather than clearing it, since secrets are never echoed
// back on read — so a partial edit cannot accidentally wipe a credential.
func (h *Handlers) SiteIntegrationsUpdate(ctx context.Context, req *siteapi.UpdateIntegrationInput, params siteapi.SiteIntegrationsUpdateParams) (*siteapi.SiteIntegrationResource, error) {
	in, err := inputs.IntegrationUpdate(req)
	if err != nil {
		return nil, err
	}
	v, err := h.integrations.Update(ctx, workspaceScope(ctx), int64(params.ID), in)
	if err != nil {
		return nil, err
	}
	res := integrationResource(v)
	return &res, nil
}

// SiteIntegrationsRegenerateKey gives the integration a new hook key; the old hook URL
// stops working at once. Owners and admins only.
func (h *Handlers) SiteIntegrationsRegenerateKey(ctx context.Context, params siteapi.SiteIntegrationsRegenerateKeyParams) (*siteapi.SiteIntegrationResource, error) {
	v, err := h.integrations.RegenerateKey(ctx, workspaceScope(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := integrationResource(v)
	return &res, nil
}

// SitePresetsList returns the Presets the Instance offers with their params schemas.
func (h *Handlers) SitePresetsList(_ context.Context) ([]siteapi.SitePresetResource, error) {
	choosable := h.integrations.Choosable()
	out := make([]siteapi.SitePresetResource, 0, len(choosable))
	for _, p := range choosable {
		out = append(out, siteapi.SitePresetResource{
			Name: p.Name(), Description: p.Description(),
			ParamsSchema: siteapi.JsonObject(integrations.RawOf(p.ParamsSchema())),
		})
	}
	return out, nil
}

// SiteIntegrationsDelete deletes an integration for good (ADR 0049): it leaves the list and gives
// up the default, unless live work uses it (409). Owners and admins only.
func (h *Handlers) SiteIntegrationsDelete(ctx context.Context, params siteapi.SiteIntegrationsDeleteParams) error {
	return h.integrationDeletion.Delete(ctx, workspaceScope(ctx), int64(params.ID))
}

// integrationResource maps the Integration's View: the row with goverter, then what the module
// built beside it (the redacted config and params, the hook URL and the Provider limit).
func integrationResource(v integrations.View) siteapi.SiteIntegrationResource {
	res := mapper.IntegrationToResource(v.Row)
	res.ProviderLimit = mapper.ProviderLimitToStatus(v.ProviderLimit)
	res.HookUrl = v.HookURL
	res.Params = siteapi.JsonObject(v.Params)
	res.SecretsRetiring = v.SecretsRetiring
	// The config is a oneOf per provider, which goverter cannot express.
	cfg := v.Config
	switch {
	case cfg.SMTP != nil:
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteSmtpConfigSiteIntegrationConfigSum(mapper.SmtpConfigToResource(*cfg.SMTP))}
	case cfg.SES != nil:
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteSesConfigSiteIntegrationConfigSum(mapper.SesConfigToResource(*cfg.SES))}
	case cfg.Inbox != nil:
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteInboxConfigSiteIntegrationConfigSum(siteapi.SiteInboxConfig{Kind: siteapi.SiteInboxConfigKindInbox})}
	case cfg.Telegram != nil:
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteTelegramConfigSiteIntegrationConfigSum(mapper.TelegramConfigToResource(*cfg.Telegram))}
	case cfg.Twilio != nil:
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteTwilioConfigSiteIntegrationConfigSum(mapper.TwilioConfigToResource(*cfg.Twilio))}
	}
	return res
}
