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
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/sendlimit"
)

// The Integration operations are thin adapters over internal/integrations, the module
// /site shares: validation, credential sealing and the default-per-channel rule live
// there. Credentials are write-only; every response carries the config with secrets
// redacted.

// IntegrationsList returns the Workspace's sending-provider Integrations with the Send
// rate limit as enforced and the last 24 hours of usage.
func (h *Handlers) IntegrationsList(ctx context.Context, params externalapi.IntegrationsListParams) (externalapi.IntegrationsListRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "integrations:read") {
		res := externalapi.IntegrationsListUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)
	page, err := h.integrations.List(ctx, s, pagination.ParamsOf(params.Page, params.PageSize))
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
		Page:       int32(page.Page),
		PageSize:   int32(page.PageSize),
		TotalItems: int32(page.TotalItems),
		TotalPages: int32(page.TotalPages),
	}, nil
}

// IntegrationsCreate stores a new Integration; credentials are sealed and never returned.
func (h *Handlers) IntegrationsCreate(ctx context.Context, req *externalapi.CreateIntegrationInput) (externalapi.IntegrationsCreateRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "integrations:write") {
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
	var verr *integrations.ValidationError
	switch {
	case errors.As(err, &verr):
		res := externalapi.IntegrationsCreateUnprocessableEntity(validationProblem(verr))
		return &res, nil
	case errors.Is(err, integrations.ErrDefaultConflict):
		res := externalapi.IntegrationsCreateConflict(problem(http.StatusConflict, err.Error()))
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
	if !auth.HasScope(auth.GetTokenAuth(ctx), "integrations:read") {
		res := externalapi.IntegrationsGetUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	s := auth.TokenScoped(ctx)
	row, err := s.Integration().Get(ctx, id)
	if ent.IsNotFound(err) {
		res := externalapi.IntegrationsGetNotFound(problem(http.StatusNotFound, "integration not found"))
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
	if !auth.HasScope(auth.GetTokenAuth(ctx), "integrations:write") {
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
	var verr *integrations.ValidationError
	switch {
	case ent.IsNotFound(err):
		res := externalapi.IntegrationsUpdateNotFound(problem(http.StatusNotFound, "integration not found"))
		return &res, nil
	case errors.As(err, &verr):
		res := externalapi.IntegrationsUpdateUnprocessableEntity(validationProblem(verr))
		return &res, nil
	case errors.Is(err, integrations.ErrDefaultConflict):
		res := externalapi.IntegrationsUpdateConflict(problem(http.StatusConflict, err.Error()))
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
	if !auth.HasScope(auth.GetTokenAuth(ctx), "integrations:write") {
		res := externalapi.IntegrationsRegenerateKeyUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	s := auth.TokenScoped(ctx)
	row, err := h.integrations.RegenerateKey(ctx, s, int64(params.ID))
	if ent.IsNotFound(err) {
		res := externalapi.IntegrationsRegenerateKeyNotFound(problem(http.StatusNotFound, "integration not found"))
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
	if !auth.HasScope(auth.GetTokenAuth(ctx), "integrations:read") {
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
		Items: items, Page: 1, PageSize: int32(len(items)), TotalItems: int32(len(items)), TotalPages: 1,
	}, nil
}

// IntegrationsDelete removes an Integration.
func (h *Handlers) IntegrationsDelete(ctx context.Context, params externalapi.IntegrationsDeleteParams) (externalapi.IntegrationsDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "integrations:write") {
		res := externalapi.IntegrationsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	err := h.integrations.Delete(ctx, auth.TokenScoped(ctx), id)
	if refused, ok := inuse.As(err); ok {
		res := inUseProblem(refused)
		return &res, nil
	}
	if ent.IsNotFound(err) {
		res := externalapi.IntegrationsDeleteNotFound(problem(http.StatusNotFound, "integration not found"))
		return &res, nil
	}
	if err != nil {
		return nil, err
	}
	return &externalapi.IntegrationsDeleteNoContent{}, nil
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
	}
	return integrations.ConfigInput{}, errors.New("unsupported provider")
}

func validationProblem(v *integrations.ValidationError) externalapi.ProblemDetails {
	p := problem(http.StatusUnprocessableEntity, v.Message)
	p.Errors = externalapi.NewOptProblemDetailsErrors(externalapi.ProblemDetailsErrors{v.Field: {v.Message}})
	return p
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
	// The config is a oneOf per provider, which goverter cannot express.
	switch {
	case cfg.SMTP != nil:
		c := cfg.SMTP
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewSmtpConfigIntegrationConfigSum(externalapi.SmtpConfig{
			Kind: externalapi.SmtpConfigKindSMTP, Host: c.Host, Port: int32(c.Port), From: externalapi.EmailAddress(c.From),
			Username: omitEmpty(c.Username), FromName: omitEmpty(c.FromName),
		})}
	case cfg.SES != nil:
		c := cfg.SES
		res.Config = externalapi.IntegrationConfig{OneOf: externalapi.NewSesConfigIntegrationConfigSum(externalapi.SesConfig{
			Kind: externalapi.SesConfigKindSes, Region: c.Region, From: externalapi.EmailAddress(c.From),
			FromName: omitEmpty(c.FromName), Endpoint: omitEmpty(c.Endpoint),
			AccessKeyIdLast4: omitEmpty(c.AccessKeyIDLast4),
		})}
	}
	return res, nil
}

// omitEmpty is unset for an empty value, so redacted optional fields are omitted.
func omitEmpty(s string) externalapi.OptNilString {
	if s == "" {
		return externalapi.OptNilString{}
	}
	return externalapi.NewOptNilString(s)
}
