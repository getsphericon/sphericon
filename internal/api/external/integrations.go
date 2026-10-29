package external

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/narrow"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/sendlimit"
	"github.com/getsphericon/sphericon/internal/validation"
)

// The Integration operations are thin adapters over internal/integrations, the module
// /site shares: validation, credential sealing and the default-per-channel rule live
// there. Credentials are write-only; every response carries the config with secrets
// redacted.

// IntegrationsList returns the Workspace's sending-provider Integrations with the Send
// rate limit as enforced and the last 24 hours of usage.
func (h *Handlers) IntegrationsList(ctx context.Context, params externalapi.IntegrationsListParams) (externalapi.IntegrationsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeIntegrationsRead) {
		res := externalapi.IntegrationsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)
	page, err := h.integrations.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize), params.Archived.Or(false))
	if err != nil {
		return nil, err
	}
	limits, err := sendlimit.Statuses(ctx, s, page.Items, time.Now())
	if err != nil {
		return nil, err
	}
	items := make([]externalapi.IntegrationResource, len(page.Items))
	for i, row := range page.Items {
		if items[i], err = h.integrationResource(row, limits[row.ID]); err != nil {
			return nil, err
		}
	}
	return &externalapi.IntegrationsListOK{
		Items:      items,
		Page:       narrow.Int32(page.Page),
		PageSize:   narrow.Int32(page.PageSize),
		TotalItems: narrow.Int32(page.TotalItems),
		TotalPages: narrow.Int32(page.TotalPages),
	}, nil
}

// IntegrationsCreate stores a new Integration; credentials are sealed and never returned.
func (h *Handlers) IntegrationsCreate(ctx context.Context, req *externalapi.CreateIntegrationInput) (externalapi.IntegrationsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeIntegrationsWrite) {
		res := externalapi.IntegrationsCreateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	cfg, err := configInput(req.Config)
	if err != nil {
		return nil, err
	}
	s := auth.TokenScoped(ctx)
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
	switch {
	case integrations.IsConflict(err):
		res := externalapi.IntegrationsCreateConflict(problem(http.StatusConflict, validation.Message(err)))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res, err := h.integrationOne(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// IntegrationsGet returns one Integration with secrets redacted.
func (h *Handlers) IntegrationsGet(ctx context.Context, params externalapi.IntegrationsGetParams) (externalapi.IntegrationsGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeIntegrationsRead) {
		res := externalapi.IntegrationsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	row, err := h.integrations.Get(ctx, s, id)
	if errors.Is(err, integrations.ErrNotFound) {
		res := externalapi.IntegrationsGetNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res, err := h.integrationOne(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// IntegrationsUpdate edits an Integration. Omitting config keeps the stored credentials;
// blank secret fields inside a supplied config keep the stored secret.
func (h *Handlers) IntegrationsUpdate(ctx context.Context, req *externalapi.UpdateIntegrationInput, params externalapi.IntegrationsUpdateParams) (externalapi.IntegrationsUpdateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeIntegrationsWrite) {
		res := externalapi.IntegrationsUpdateUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
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
	s := auth.TokenScoped(ctx)
	var row *ent.Integration
	inParams, err := integrations.ParamsOf(req.Params.Or(nil))
	if err == nil {
		in.Params = inParams
		row, err = h.integrations.Update(ctx, s, id, in)
	}
	switch {
	case errors.Is(err, integrations.ErrNotFound):
		res := externalapi.IntegrationsUpdateNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	case integrations.IsConflict(err):
		res := externalapi.IntegrationsUpdateConflict(problem(http.StatusConflict, validation.Message(err)))
		return &res, nil
	case err != nil:
		return nil, err
	}
	res, err := h.integrationOne(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// IntegrationsRegenerateKey gives the Integration a new hook key: the old hook URL stops
// working at once. Hidden from MCP, because it breaks a working URL and needs a person.
func (h *Handlers) IntegrationsRegenerateKey(ctx context.Context, params externalapi.IntegrationsRegenerateKeyParams) (externalapi.IntegrationsRegenerateKeyRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeIntegrationsWrite) {
		res := externalapi.IntegrationsRegenerateKeyUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)
	row, err := h.integrations.RegenerateKey(ctx, s, int64(params.ID))
	if errors.Is(err, integrations.ErrNotFound) {
		res := externalapi.IntegrationsRegenerateKeyNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	res, err := h.integrationOne(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// PresetsList returns the Presets the Instance offers with the params schema of each:
// how an agent learns what an Integration's `params` takes.
func (h *Handlers) PresetsList(ctx context.Context) (externalapi.PresetsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeIntegrationsRead) {
		res := problem(http.StatusUnauthorized, "insufficient scope")
		return &res, nil
	}
	reg := h.integrations.Presets()
	names := reg.Names()
	items := make([]externalapi.PresetResource, 0, len(names))
	for _, name := range names {
		p, _ := reg.Get(name)
		items = append(items, externalapi.PresetResource{
			Name: p.Name(), Description: p.Description(),
			ParamsSchema: externalapi.PresetResourceParamsSchema(integrations.RawOf(p.ParamsSchema())),
		})
	}
	return &externalapi.PresetsListOK{
		Items: items, Page: 1, PageSize: narrow.Int32(len(items)), TotalItems: narrow.Int32(len(items)), TotalPages: 1,
	}, nil
}

// IntegrationsArchive archives an Integration (ADR 0049): it leaves the list and gives up the
// default, unless live work uses it (409).
func (h *Handlers) IntegrationsArchive(ctx context.Context, params externalapi.IntegrationsArchiveParams) (externalapi.IntegrationsArchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeIntegrationsWrite) {
		res := externalapi.IntegrationsArchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)
	id := int64(params.ID)
	err := h.integrations.Archive(ctx, s, id)
	if errors.Is(err, integrations.ErrNotFound) {
		res := externalapi.IntegrationsArchiveNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return h.archivedOrRestored(ctx, s, id)
}

// IntegrationsUnarchive restores an archived Integration to the list. It is not the default.
func (h *Handlers) IntegrationsUnarchive(ctx context.Context, params externalapi.IntegrationsUnarchiveParams) (externalapi.IntegrationsUnarchiveRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), externalapi.ApiTokenScopeIntegrationsWrite) {
		res := externalapi.IntegrationsUnarchiveUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)
	id := int64(params.ID)
	err := h.integrations.Unarchive(ctx, s, id)
	switch {
	case errors.Is(err, integrations.ErrNotFound):
		res := externalapi.IntegrationsUnarchiveNotFound(problem(http.StatusNotFound, validation.Message(err)))
		return &res, nil
	case integrations.IsConflict(err):
		res := externalapi.IntegrationsUnarchiveConflict(problem(http.StatusConflict, validation.Message(err)))
		return &res, nil
	case err != nil:
		return nil, err
	}
	return h.archivedOrRestored(ctx, s, id)
}

// archivedOrRestored reads the Integration back after an archive or an unarchive.
func (h *Handlers) archivedOrRestored(ctx context.Context, s *ent.Scoped, id int64) (*externalapi.IntegrationResource, error) {
	row, err := h.integrations.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	res, err := h.integrationOne(ctx, s, row)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// configInput maps the typed config union onto the module's provider-neutral input.
func configInput(in externalapi.IntegrationConfigInput) (integrations.ConfigInput, error) {
	switch in.OneOf.Type {
	case externalapi.SmtpConfigInputIntegrationConfigInputSum:
		c := in.OneOf.SmtpConfigInput
		return integrations.ConfigInput{SMTP: &integrations.SMTPConfig{
			Host: c.Host, Port: int(c.Port), Username: c.Username.Or(""), Password: c.Password.Or(""),
			From: string(c.From), FromName: c.FromName.Or(""),
		}}, nil
	case externalapi.SesConfigInputIntegrationConfigInputSum:
		c := in.OneOf.SesConfigInput
		return integrations.ConfigInput{SES: &integrations.SESConfig{
			Region: c.Region, AccessKeyID: c.AccessKeyId, SecretAccessKey: c.SecretAccessKey,
			From: string(c.From), FromName: c.FromName.Or(""), Endpoint: c.Endpoint.Or(""),
		}}, nil
	case externalapi.InboxConfigInputIntegrationConfigInputSum:
		return integrations.ConfigInput{Inbox: &integrations.InboxConfig{}}, nil
	}
	return integrations.ConfigInput{}, errors.New("unsupported provider")
}

// integrationOne renders a single row with its 24-hour usage.
func (h *Handlers) integrationOne(ctx context.Context, s *ent.Scoped, row *ent.Integration) (externalapi.IntegrationResource, error) {
	limits, err := sendlimit.Statuses(ctx, s, []*ent.Integration{row}, time.Now())
	if err != nil {
		return externalapi.IntegrationResource{}, err
	}
	return h.integrationResource(row, limits[row.ID])
}

// integrationResource maps the row with goverter, then attaches what the mapper cannot
// see: the redacted config and params (secrets never leave the module), the hook URL and
// the Send rate limit.
func (h *Handlers) integrationResource(row *ent.Integration, limit sendlimit.Status) (externalapi.IntegrationResource, error) {
	cfg, err := h.integrations.Redact(row)
	if err != nil {
		return externalapi.IntegrationResource{}, err
	}
	view, err := h.integrations.View(row)
	if err != nil {
		return externalapi.IntegrationResource{}, err
	}
	res := mapper.IntegrationToResource(row)
	res.SendLimit = mapper.SendLimitToStatus(limit)
	res.Params = externalapi.IntegrationResourceParams(integrations.RawOf(view.Values))
	res.SecretsRetiring = view.SecretsRetiring
	res.HookUrl = h.integrations.HookURL(row)
	// The config is a oneOf per provider, which goverter cannot express: only the wrapper
	// is hand-written, the members are mapped by goverter.
	switch {
	case cfg.SMTP != nil:
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewSmtpConfigIntegrationConfigSum(mapper.SmtpConfigToResource(*cfg.SMTP))}
	case cfg.SES != nil:
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewSesConfigIntegrationConfigSum(mapper.SesConfigToResource(*cfg.SES))}
	case cfg.Inbox != nil:
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewInboxConfigIntegrationConfigSum(externalapi.InboxConfig{Kind: externalapi.InboxConfigKindInbox})}
	}
	return res, nil
}
