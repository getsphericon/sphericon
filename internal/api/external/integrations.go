package external

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/integrations"
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
	usage, err := sendlimit.Usage(ctx, s, time.Now())
	if err != nil {
		return nil, err
	}
	items := make([]externalapi.IntegrationResource, len(page.Items))
	for i, row := range page.Items {
		if items[i], err = h.integrationResource(row, usage); err != nil {
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
	row, err := h.integrations.Create(ctx, s, integrations.CreateInput{
		Name:         req.Name,
		Enabled:      convert.Ptr(req.Enabled),
		IsDefault:    req.IsDefault.Or(false),
		MaxPerSecond: integrations.LimitOf(req.MaxPerSecond),
		MaxPerDay:    integrations.LimitOf(req.MaxPerDay),
		Config:       cfg,
	})
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
		Name:         convert.Ptr(req.Name),
		Enabled:      convert.Ptr(req.Enabled),
		IsDefault:    convert.Ptr(req.IsDefault),
		MaxPerSecond: integrations.LimitOf(req.MaxPerSecond),
		MaxPerDay:    integrations.LimitOf(req.MaxPerDay),
	}
	if c, ok := req.Config.Get(); ok {
		cfg, err := configInput(c)
		if err != nil {
			return nil, err
		}
		in.Config = &cfg
	}
	s := auth.TokenScoped(ctx)
	row, err := h.integrations.Update(ctx, s, id, in)
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

// IntegrationsDelete removes an Integration.
func (h *Handlers) IntegrationsDelete(ctx context.Context, params externalapi.IntegrationsDeleteParams) (externalapi.IntegrationsDeleteRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "integrations:write") {
		res := externalapi.IntegrationsDeleteUnauthorized(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	id := int64(params.ID)
	err := h.integrations.Delete(ctx, auth.TokenScoped(ctx), id)
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
	usage, err := sendlimit.Usage(ctx, s, time.Now())
	if err != nil {
		return externalapi.IntegrationResource{}, err
	}
	return h.integrationResource(row, usage)
}

// integrationResource is hand-built (not goverter): the Send rate limit is computed
// from the Integration and its usage, and the config is the redacted view of the
// sealed one. Secrets never leave the module.
func (h *Handlers) integrationResource(row *ent.Integration, usage map[int64]int) (externalapi.IntegrationResource, error) {
	cfg, err := h.integrations.Redact(row)
	if err != nil {
		return externalapi.IntegrationResource{}, err
	}
	res := externalapi.IntegrationResource{
		ID:           externalapi.EntityId(row.ID),
		Name:         row.Name,
		Channel:      externalapi.IntegrationChannel(row.Channel.String()),
		Provider:     externalapi.IntegrationProvider(row.Provider.String()),
		Enabled:      row.Enabled,
		IsDefault:    row.IsDefault,
		MaxPerSecond: limitOpt(row.MaxPerSecond),
		MaxPerDay:    limitOpt(row.MaxPerDay),
		SendLimit:    sendLimitStatus(row, usage),
		CreatedAt:    externalapi.Timestamp(row.CreatedAt),
		UpdatedAt:    externalapi.Timestamp(row.UpdatedAt),
	}
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

// limitOpt renders a stored limit: null when there is none.
func limitOpt(v *int) externalapi.NilInt32 {
	if v == nil {
		return externalapi.NilInt32{Null: true}
	}
	return externalapi.NewNilInt32(int32(*v))
}

// omitEmpty is unset for an empty value, so redacted optional fields are omitted.
func omitEmpty(s string) externalapi.OptNilString {
	if s == "" {
		return externalapi.OptNilString{}
	}
	return externalapi.NewOptNilString(s)
}

func sendLimitStatus(row *ent.Integration, usage map[int64]int) externalapi.SendLimitStatus {
	eff := sendlimit.EffectiveOf(row)
	out := externalapi.SendLimitStatus{
		PerSecond:   sendLimitValue(eff.PerSecond),
		PerDay:      sendLimitValue(eff.PerDay),
		SentLast24h: int32(usage[row.ID]),
		Warnings:    lo.Map(eff.Warnings(), func(w sendlimit.Warning, _ int) externalapi.SendLimitWarning { return externalapi.SendLimitWarning(w) }),
	}
	return out
}

func sendLimitValue(v sendlimit.Value) externalapi.SendLimitValue {
	out := externalapi.SendLimitValue{}
	if v.Limit == nil {
		out.Limit = externalapi.NilInt32{Null: true}
		out.Source = externalapi.NilSendLimitSource{Null: true}
		return out
	}
	out.Limit = externalapi.NewNilInt32(int32(*v.Limit))
	out.Source = externalapi.NewNilSendLimitSource(externalapi.SendLimitSource(v.Source))
	return out
}
