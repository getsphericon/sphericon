package site

import (
	"context"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/sendlimit"
)

// SiteIntegrationsList returns the workspace's sending-provider integrations
// with secrets redacted.
func (h *Handlers) SiteIntegrationsList(ctx context.Context, params siteapi.SiteIntegrationsListParams) (siteapi.SiteIntegrationsListRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	rows, err := h.integrations.All(ctx, s, params.Archived.Or(false))
	if err != nil {
		return nil, err
	}

	limits, err := sendlimit.Statuses(ctx, s, rows, time.Now())
	if err != nil {
		return nil, err
	}
	items := make(siteapi.SiteIntegrationsListOKApplicationJSON, len(rows))
	for i, row := range rows {
		res, err := h.integrationToResource(row, limits[row.ID])
		if err != nil {
			return nil, err
		}
		items[i] = res
	}
	return &items, nil
}

// SiteIntegrationsCreate stores a new provider integration; credentials are
// encrypted at rest and never returned. Owners and admins only.
func (h *Handlers) SiteIntegrationsCreate(ctx context.Context, req *siteapi.SiteCreateIntegrationInput, params siteapi.SiteIntegrationsCreateParams) (siteapi.SiteIntegrationsCreateRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermIntegrationsManage); err != nil {
		return nil, err
	}

	cfg, err := configInput(req.Config)
	if err != nil {
		return nil, err
	}
	inParams, err := integrations.ParamsOf(req.Params.Or(nil))
	var row *ent.Integration
	if err == nil {
		row, err = h.integrations.Create(ctx, s, integrations.CreateInput{
			Name:         req.Name,
			Enabled:      convert.Ptr(req.Enabled),
			IsDefault:    req.IsDefault.Or(false),
			MaxPerSecond: integrations.LimitOf(req.MaxPerSecond),
			MaxPerDay:    integrations.LimitOf(req.MaxPerDay),
			Config:       cfg,
			Preset:       integrations.PresetOf(req.Preset).Name,
			Params:       inParams,
		})
	}
	if err != nil {
		return nil, err
	}
	res, err := h.integrationResource(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteIntegrationsGet returns one integration with secrets redacted.
func (h *Handlers) SiteIntegrationsGet(ctx context.Context, params siteapi.SiteIntegrationsGetParams) (siteapi.SiteIntegrationsGetRes, error) {
	s, err := h.scopedFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}

	id := int64(params.ID)
	row, err := h.integrations.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res, err := h.integrationResource(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteIntegrationsUpdate edits an integration. Omitting config keeps the stored
// credentials; a supplied config must match the integration's provider kind.
// Within a supplied config, blank secret fields (password, secret access key)
// keep the stored secret rather than clearing it, since secrets are never echoed
// back on read — so a partial edit cannot accidentally wipe a credential.
func (h *Handlers) SiteIntegrationsUpdate(ctx context.Context, req *siteapi.SiteUpdateIntegrationInput, params siteapi.SiteIntegrationsUpdateParams) (siteapi.SiteIntegrationsUpdateRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermIntegrationsManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)

	in := integrations.UpdateInput{
		Name:          convert.Ptr(req.Name),
		Enabled:       convert.Ptr(req.Enabled),
		IsDefault:     convert.Ptr(req.IsDefault),
		MaxPerSecond:  integrations.LimitOf(req.MaxPerSecond),
		MaxPerDay:     integrations.LimitOf(req.MaxPerDay),
		Preset:        integrations.PresetOf(req.Preset),
		RetireSecrets: req.RetireSecrets.Or(false),
	}
	if c, ok := req.Config.Get(); ok {
		cfg, err := configInput(c)
		if err != nil {
			return nil, err
		}
		in.Config = &cfg
	}
	var updated *ent.Integration
	inParams, err := integrations.ParamsOf(req.Params.Or(nil))
	if err == nil {
		in.Params = inParams
		updated, err = h.integrations.Update(ctx, s, id, in)
	}
	if err != nil {
		return nil, err
	}
	res, err := h.integrationResource(ctx, s, updated)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteIntegrationsRegenerateKey gives the integration a new hook key; the old hook URL
// stops working at once. Owners and admins only.
func (h *Handlers) SiteIntegrationsRegenerateKey(ctx context.Context, params siteapi.SiteIntegrationsRegenerateKeyParams) (siteapi.SiteIntegrationsRegenerateKeyRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermIntegrationsManage); err != nil {
		return nil, err
	}
	row, err := h.integrations.RegenerateKey(ctx, s, int64(params.ID))
	if err != nil {
		return nil, err
	}
	res, err := h.integrationResource(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SitePresetsList returns the Presets the Instance offers with their params schemas.
func (h *Handlers) SitePresetsList(_ context.Context) ([]siteapi.SitePresetResource, error) {
	reg := h.integrations.Presets()
	out := make([]siteapi.SitePresetResource, 0, len(reg.Names()))
	for _, name := range reg.Names() {
		p, _ := reg.Get(name)
		out = append(out, siteapi.SitePresetResource{
			Name: p.Name(), Description: p.Description(),
			ParamsSchema: siteapi.SitePresetResourceParamsSchema(integrations.RawOf(p.ParamsSchema())),
		})
	}
	return out, nil
}

// SiteIntegrationsArchive archives an integration (ADR 0049): it leaves the list and gives up the
// default, unless live work uses it (409). Owners and admins only.
func (h *Handlers) SiteIntegrationsArchive(ctx context.Context, params siteapi.SiteIntegrationsArchiveParams) (siteapi.SiteIntegrationsArchiveRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermIntegrationsManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.integrations.Archive(ctx, s, id)
	if err != nil {
		return nil, err
	}
	row, err := h.integrations.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res, err := h.integrationResource(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteIntegrationsUnarchive restores an archived integration to the list. It is not the default.
// Owners and admins only.
func (h *Handlers) SiteIntegrationsUnarchive(ctx context.Context, params siteapi.SiteIntegrationsUnarchiveParams) (siteapi.SiteIntegrationsUnarchiveRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.WorkspaceId)
	if err != nil {
		return nil, err
	}
	if err := accounts.Require(role, accounts.PermIntegrationsManage); err != nil {
		return nil, err
	}

	id := int64(params.ID)
	err = h.integrations.Unarchive(ctx, s, id)
	if err != nil {
		return nil, err
	}
	row, err := h.integrations.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res, err := h.integrationResource(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// --- helpers ---

// configInput maps the typed config union onto the module's provider-neutral input.
func configInput(in siteapi.SiteIntegrationConfigInput) (integrations.ConfigInput, error) {
	switch in.OneOf.Type {
	case siteapi.SiteSmtpConfigInputSiteIntegrationConfigInputSum:
		c := in.OneOf.SiteSmtpConfigInput
		return integrations.ConfigInput{SMTP: &integrations.SMTPConfig{
			Host: c.Host, Port: int(c.Port), Username: c.Username.Or(""), Password: c.Password.Or(""),
			From: string(c.From), FromName: c.FromName.Or(""),
		}}, nil
	case siteapi.SiteSesConfigInputSiteIntegrationConfigInputSum:
		c := in.OneOf.SiteSesConfigInput
		return integrations.ConfigInput{SES: &integrations.SESConfig{
			Region: c.Region, AccessKeyID: c.AccessKeyId, SecretAccessKey: c.SecretAccessKey,
			From: string(c.From), FromName: c.FromName.Or(""), Endpoint: c.Endpoint.Or(""),
		}}, nil
	case siteapi.SiteInboxConfigInputSiteIntegrationConfigInputSum:
		return integrations.ConfigInput{Inbox: &integrations.InboxConfig{}}, nil
	}
	return integrations.ConfigInput{}, errUnknownProvider
}

var errUnknownProvider = errors.New("unsupported provider")

// integrationToResource maps the row with goverter, then attaches what the mapper cannot
// see: the decrypted config and params (redacted), the hook URL and the Send rate limit.
func (h *Handlers) integrationToResource(row *ent.Integration, limit sendlimit.Status) (siteapi.SiteIntegrationResource, error) {
	res := mapper.IntegrationToResource(row)
	res.SendLimit = mapper.SendLimitToStatus(limit)
	res.HookUrl = h.integrations.HookURL(row)
	view, err := h.integrations.View(row)
	if err != nil {
		return res, err
	}
	res.Params = siteapi.SiteIntegrationResourceParams(integrations.RawOf(view.Values))
	res.SecretsRetiring = view.SecretsRetiring

	cfg, err := h.integrations.Redact(row)
	if err != nil {
		return res, err
	}
	// The config is a oneOf per provider, which goverter cannot express.
	switch {
	case cfg.SMTP != nil:
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteSmtpConfigSiteIntegrationConfigSum(mapper.SmtpConfigToResource(*cfg.SMTP))}
	case cfg.SES != nil:
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteSesConfigSiteIntegrationConfigSum(mapper.SesConfigToResource(*cfg.SES))}
	case cfg.Inbox != nil:
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteInboxConfigSiteIntegrationConfigSum(siteapi.SiteInboxConfig{Kind: siteapi.SiteInboxConfigKindInbox})}
	}
	return res, nil
}

// integrationResource renders one Integration with its 24-hour usage, for the
// single-row responses (get, create, update).
func (h *Handlers) integrationResource(ctx context.Context, s *ent.Scoped, row *ent.Integration) (siteapi.SiteIntegrationResource, error) {
	limits, err := sendlimit.Statuses(ctx, s, []*ent.Integration{row}, time.Now())
	if err != nil {
		return siteapi.SiteIntegrationResource{}, err
	}
	return h.integrationToResource(row, limits[row.ID])
}
