package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/domain/sending/integrations"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
)

// The Integration operations map internal/domain/sending/integrations, the module /site shares:
// validation, credential sealing, the default-per-channel rule and the View every response is
// built from live there. Credentials are write-only; every response carries the config with
// secrets redacted.

// IntegrationsList returns the Workspace's sending-provider Integrations with the Send
// rate limit as enforced and the last 24 hours of usage.
func (h *Handlers) IntegrationsList(ctx context.Context, params externalapi.IntegrationsListParams) (*externalapi.IntegrationsListOK, error) {
	page, err := h.integrations.List(ctx, auth.TokenScoped(ctx), pagination.ParamsOf(params.Page, params.PageSize))
	if err != nil {
		return nil, err
	}
	return mapper.IntegrationsListPage(pagination.Map(page, integrationResource)), nil
}

// IntegrationsCreate stores a new Integration; credentials are sealed and never returned.
func (h *Handlers) IntegrationsCreate(ctx context.Context, req *externalapi.CreateIntegrationInput) (*externalapi.IntegrationResource, error) {
	in, err := inputs.IntegrationCreate(req)
	if err != nil {
		return nil, err
	}
	v, err := h.integrations.Create(ctx, auth.TokenScoped(ctx), in)
	if err != nil {
		return nil, err
	}
	res := integrationResource(v)
	return &res, nil
}

// IntegrationsGet returns one Integration with secrets redacted.
func (h *Handlers) IntegrationsGet(ctx context.Context, params externalapi.IntegrationsGetParams) (*externalapi.IntegrationResource, error) {
	v, err := h.integrations.View(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := integrationResource(v)
	return &res, nil
}

// IntegrationsUpdate edits an Integration. Omitting config keeps the stored credentials;
// blank secret fields inside a supplied config keep the stored secret.
func (h *Handlers) IntegrationsUpdate(ctx context.Context, req *externalapi.UpdateIntegrationInput, params externalapi.IntegrationsUpdateParams) (*externalapi.IntegrationResource, error) {
	in, err := inputs.IntegrationUpdate(req)
	if err != nil {
		return nil, err
	}
	v, err := h.integrations.Update(ctx, auth.TokenScoped(ctx), int64(params.ID), in)
	if err != nil {
		return nil, err
	}
	res := integrationResource(v)
	return &res, nil
}

// IntegrationsRegenerateKey gives the Integration a new hook key: the old hook URL stops
// working at once. Hidden from MCP, because it breaks a working URL and needs a person.
func (h *Handlers) IntegrationsRegenerateKey(ctx context.Context, params externalapi.IntegrationsRegenerateKeyParams) (*externalapi.IntegrationResource, error) {
	v, err := h.integrations.RegenerateKey(ctx, auth.TokenScoped(ctx), int64(params.ID))
	if err != nil {
		return nil, err
	}
	res := integrationResource(v)
	return &res, nil
}

// PresetsList returns the Presets the Instance offers with the params schema of each:
// how an agent learns what an Integration's `params` takes.
func (h *Handlers) PresetsList(_ context.Context) (*externalapi.PresetsListOK, error) {
	choosable := h.integrations.Choosable()
	items := make([]externalapi.PresetResource, 0, len(choosable))
	for _, p := range choosable {
		items = append(items, externalapi.PresetResource{
			Name: p.Name(), Description: p.Description(),
			ParamsSchema: externalapi.JsonObject(integrations.RawOf(p.ParamsSchema())),
		})
	}
	return mapper.PresetsListPage(pagination.Whole(items)), nil
}

// IntegrationsDelete deletes an Integration for good (ADR 0049): it leaves the list and gives up
// the default, unless live work uses it (409).
func (h *Handlers) IntegrationsDelete(ctx context.Context, params externalapi.IntegrationsDeleteParams) error {
	return h.integrationDeletion.Delete(ctx, auth.TokenScoped(ctx), int64(params.ID))
}

// integrationResource maps the Integration's View: the row with goverter, then what the module
// built beside it (the redacted config and params, the hook URL and the Provider limit).
func integrationResource(v integrations.View) externalapi.IntegrationResource {
	res := mapper.IntegrationToResource(v.Row)
	res.ProviderLimit = mapper.ProviderLimitToStatus(v.ProviderLimit)
	res.Params = externalapi.JsonObject(v.Params)
	res.SecretsRetiring = v.SecretsRetiring
	res.HookUrl = v.HookURL
	// The config is a oneOf per provider, which goverter cannot express: only the wrapper
	// is hand-written, the members are mapped by goverter.
	cfg := v.Config
	switch {
	case cfg.SMTP != nil:
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewSmtpConfigIntegrationConfigSum(mapper.SmtpConfigToResource(*cfg.SMTP))}
	case cfg.SES != nil:
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewSesConfigIntegrationConfigSum(mapper.SesConfigToResource(*cfg.SES))}
	case cfg.Inbox != nil:
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewInboxConfigIntegrationConfigSum(externalapi.InboxConfig{Kind: externalapi.InboxConfigKindInbox})}
	case cfg.Telegram != nil:
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewTelegramConfigIntegrationConfigSum(mapper.TelegramConfigToResource(*cfg.Telegram))}
	case cfg.Twilio != nil:
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewTwilioConfigIntegrationConfigSum(mapper.TwilioConfigToResource(*cfg.Twilio))}
	}
	return res
}
