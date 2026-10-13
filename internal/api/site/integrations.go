package site

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	siteapi "github.com/getsphericon/sphericon/gen/site"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/integrations"
	"github.com/getsphericon/sphericon/internal/sendlimit"
)

const errIntegrationsForbidden = "only owners and admins can manage integrations"

// SiteIntegrationsList returns the workspace's sending-provider integrations
// with secrets redacted.
func (h *Handlers) SiteIntegrationsList(ctx context.Context, params siteapi.SiteIntegrationsListParams) (siteapi.SiteIntegrationsListRes, error) {
	s, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := problem(http.StatusNotFound, "workspace not found")
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.Integration().Query().
		Order(ent.Asc(integration.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	usage, err := sendlimit.Usage(ctx, s, time.Now())
	if err != nil {
		return nil, err
	}
	items := make(siteapi.SiteIntegrationsListOKApplicationJSON, len(rows))
	for i, row := range rows {
		res, err := h.integrationToResource(row, usage)
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
	s, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteIntegrationsCreateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageIntegrations(role) {
		v := siteapi.SiteIntegrationsCreateForbidden(problem(http.StatusForbidden, errIntegrationsForbidden))
		return &v, nil
	}

	cfg, err := configInput(req.Config)
	if err != nil {
		return nil, err
	}
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
		v := siteapi.SiteIntegrationsCreateUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity, verr.Message, map[string][]string{verr.Field: {verr.Message}},
		))
		return &v, nil
	case errors.Is(err, integrations.ErrDefaultConflict):
		v := siteapi.SiteIntegrationsCreateConflict(problem(http.StatusConflict, err.Error()))
		return &v, nil
	case err != nil:
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
	s, err := h.scopedFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteIntegrationsGetNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteIntegrationsGetBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	row, err := s.Integration().Get(ctx, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteIntegrationsGetNotFound(problem(http.StatusNotFound, "integration not found"))
		return &v, nil
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

// SiteIntegrationsUpdate edits an integration. Omitting config keeps the stored
// credentials; a supplied config must match the integration's provider kind.
// Within a supplied config, blank secret fields (password, secret access key)
// keep the stored secret rather than clearing it, since secrets are never echoed
// back on read — so a partial edit cannot accidentally wipe a credential.
func (h *Handlers) SiteIntegrationsUpdate(ctx context.Context, req *siteapi.SiteUpdateIntegrationInput, params siteapi.SiteIntegrationsUpdateParams) (siteapi.SiteIntegrationsUpdateRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteIntegrationsUpdateNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageIntegrations(role) {
		v := siteapi.SiteIntegrationsUpdateForbidden(problem(http.StatusForbidden, errIntegrationsForbidden))
		return &v, nil
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteIntegrationsUpdateBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}

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
	updated, err := h.integrations.Update(ctx, s, id, in)
	var verr *integrations.ValidationError
	switch {
	case ent.IsNotFound(err):
		r := siteapi.SiteIntegrationsUpdateNotFound(problem(http.StatusNotFound, "integration not found"))
		return &r, nil
	case errors.As(err, &verr):
		r := siteapi.SiteIntegrationsUpdateUnprocessableEntity(problemWithErrors(
			http.StatusUnprocessableEntity, verr.Message, map[string][]string{verr.Field: {verr.Message}},
		))
		return &r, nil
	case errors.Is(err, integrations.ErrDefaultConflict):
		r := siteapi.SiteIntegrationsUpdateConflict(problem(http.StatusConflict, err.Error()))
		return &r, nil
	case err != nil:
		return nil, err
	}
	res, err := h.integrationResource(ctx, s, updated)
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// SiteIntegrationsDelete removes an integration.
func (h *Handlers) SiteIntegrationsDelete(ctx context.Context, params siteapi.SiteIntegrationsDeleteParams) (siteapi.SiteIntegrationsDeleteRes, error) {
	s, role, err := h.scopedWithRoleFor(ctx, params.Slug)
	if ent.IsNotFound(err) {
		v := siteapi.SiteIntegrationsDeleteNotFound(problem(http.StatusNotFound, "workspace not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	if !accounts.CanManageIntegrations(role) {
		v := siteapi.SiteIntegrationsDeleteForbidden(problem(http.StatusForbidden, errIntegrationsForbidden))
		return &v, nil
	}

	id, err := strconv.ParseInt(string(params.ID), 10, 64)
	if err != nil {
		v := siteapi.SiteIntegrationsDeleteBadRequest(problem(http.StatusBadRequest, "invalid id"))
		return &v, nil
	}
	err = h.integrations.Delete(ctx, s, id)
	if ent.IsNotFound(err) {
		v := siteapi.SiteIntegrationsDeleteNotFound(problem(http.StatusNotFound, "integration not found"))
		return &v, nil
	}
	if err != nil {
		return nil, err
	}
	return &siteapi.SiteIntegrationsDeleteNoContent{}, nil
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
	}
	return integrations.ConfigInput{}, errUnknownProvider
}

var errUnknownProvider = errors.New("unsupported provider")

// integrationToResource decrypts the stored config and builds the API resource
// with all secrets redacted.
func (h *Handlers) integrationToResource(row *ent.Integration, usage map[int64]int) (siteapi.SiteIntegrationResource, error) {
	res := siteapi.SiteIntegrationResource{
		ID:           siteapi.EntityId(strconv.FormatInt(row.ID, 10)),
		Name:         row.Name,
		Channel:      siteapi.SiteIntegrationChannel(row.Channel.String()),
		Provider:     siteapi.SiteIntegrationProvider(row.Provider.String()),
		Enabled:      row.Enabled,
		IsDefault:    row.IsDefault,
		MaxPerSecond: limitOpt(row.MaxPerSecond),
		MaxPerDay:    limitOpt(row.MaxPerDay),
		SendLimit:    sendLimitStatus(row, usage),
		CreatedAt:    siteapi.Timestamp(row.CreatedAt),
		UpdatedAt:    siteapi.Timestamp(row.UpdatedAt),
	}

	cfg, err := h.integrations.Redact(row)
	if err != nil {
		return res, err
	}
	switch {
	case cfg.SMTP != nil:
		c := cfg.SMTP
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteSmtpConfigSiteIntegrationConfigSum(siteapi.SiteSmtpConfig{
			Kind: siteapi.SiteSmtpConfigKindSMTP, Host: c.Host, Port: int32(c.Port), From: siteapi.EmailAddress(c.From),
			Username: optNilString(c.Username), FromName: optNilString(c.FromName),
		})}
	case cfg.SES != nil:
		c := cfg.SES
		res.Config = siteapi.SiteIntegrationConfig{OneOf: siteapi.NewSiteSesConfigSiteIntegrationConfigSum(siteapi.SiteSesConfig{
			Kind: siteapi.SiteSesConfigKindSes, Region: c.Region, From: siteapi.EmailAddress(c.From),
			FromName: optNilString(c.FromName), Endpoint: optNilString(c.Endpoint),
			AccessKeyIdLast4: optNilString(c.AccessKeyIDLast4),
		})}
	}
	return res, nil
}

// limitOpt renders a stored limit: null when there is none.
func limitOpt(v *int) siteapi.NilInt32 {
	if v == nil {
		return siteapi.NilInt32{Null: true}
	}
	return siteapi.NewNilInt32(int32(*v))
}

// optNilString returns a set OptNilString for non-empty input, else the zero
// (unset) value — so redacted optional fields are omitted rather than blank.
func optNilString(s string) siteapi.OptNilString {
	if s == "" {
		return siteapi.OptNilString{}
	}
	return siteapi.NewOptNilString(s)
}

// integrationResource renders one Integration with its 24-hour usage, for the
// single-row responses (get, create, update).
func (h *Handlers) integrationResource(ctx context.Context, s *ent.Scoped, row *ent.Integration) (siteapi.SiteIntegrationResource, error) {
	usage, err := sendlimit.Usage(ctx, s, time.Now())
	if err != nil {
		return siteapi.SiteIntegrationResource{}, err
	}
	return h.integrationToResource(row, usage)
}

// sendLimitStatus renders an Integration's enforced Send rate limit with its sent
// count over the last 24 hours (from the per-Integration usage map).
func sendLimitStatus(row *ent.Integration, usage map[int64]int) siteapi.SiteSendLimitStatus {
	eff := sendlimit.EffectiveOf(row)
	out := siteapi.SiteSendLimitStatus{
		PerSecond:   sendLimitValue(eff.PerSecond),
		PerDay:      sendLimitValue(eff.PerDay),
		SentLast24h: int32(usage[row.ID]),
		Warnings:    lo.Map(eff.Warnings(), func(w sendlimit.Warning, _ int) siteapi.SiteSendLimitWarning { return siteapi.SiteSendLimitWarning(w) }),
	}
	return out
}

func sendLimitValue(v sendlimit.Value) siteapi.SiteSendLimitValue {
	out := siteapi.SiteSendLimitValue{Limit: limitOpt(v.Limit)}
	if v.Source == "" {
		out.Source = siteapi.NilSiteSendLimitSource{Null: true}
	} else {
		out.Source = siteapi.NewNilSiteSendLimitSource(siteapi.SiteSendLimitSource(v.Source))
	}
	return out
}
