package sendingdomains

import (
	"context"
	"log/slog"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/sendingdomainenvelope"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// EnvelopeState is where an email Integration's envelope domain (MAIL FROM, Return-Path) is set
// for a Sending domain, and so where SPF is checked for its mail (ADR 0184).
type EnvelopeState string

const (
	// EnvelopeProviderDefault: SES sends the domain's mail with its own MAIL FROM domain.
	EnvelopeProviderDefault EnvelopeState = "provider_default"
	// EnvelopeCustom: SES sends it with the custom MAIL FROM set on the domain's identity.
	EnvelopeCustom EnvelopeState = "custom"
	// EnvelopeUnknown: the identity could not be read, or has not been read yet.
	EnvelopeUnknown EnvelopeState = "unknown"
	// EnvelopeSetAtProvider: an SMTP relay, whose envelope is set at a provider the app does not
	// know.
	EnvelopeSetAtProvider EnvelopeState = "set_at_provider"
)

// MailFromPermission is the permission an SES Integration's credentials need to read a Sending
// domain's identity: what an unknown envelope names.
const MailFromPermission = "ses:GetEmailIdentity"

// Envelope is the envelope one email Integration sends a Sending domain's mail with, as its last
// check read it.
type Envelope struct {
	IntegrationID   int64
	IntegrationName string
	Provider        providers.Provider
	State           EnvelopeState
	// MailFrom is the custom MAIL FROM; set only when State is EnvelopeCustom.
	MailFrom *messaging.MailFrom
	// PermissionNeeded is MailFromPermission when State is EnvelopeUnknown, empty otherwise.
	PermissionNeeded string
	// CheckedAt is when the envelope was read; nil for an SMTP relay and before the first read.
	CheckedAt *time.Time
}

// Integrations is what the envelopes of a Sending domain ask of the Workspace's email
// Integrations (integrations.Module): which they are, and what a provider keeps as the domain's
// envelope.
type Integrations interface {
	EnvelopeIntegrations(ctx context.Context, s *ent.Scoped) ([]*ent.Integration, error)
	ReadMailFrom(ctx context.Context, integ *ent.Integration, domain string) (mailFrom messaging.MailFrom, keeps bool, err error)
}

// Envelopes returns the envelopes of each of doms, keyed by Sending domain id: one per email
// Integration of the Workspace, the Test inbox excepted, in the order of their ids. An SMTP relay
// is set at its provider and kept nowhere; an SES one is what the domain's last check read, and
// unknown before the first read.
func (m *Module) Envelopes(ctx context.Context, s *ent.Scoped, doms []*ent.SendingDomain) (map[int64][]Envelope, error) {
	out := make(map[int64][]Envelope, len(doms))
	if len(doms) == 0 {
		return out, nil
	}
	integs, err := m.integrations.EnvelopeIntegrations(ctx, s)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(doms))
	for i, d := range doms {
		ids[i] = d.ID
	}
	rows, err := s.SendingDomainEnvelope().Query().
		Where(sendingdomainenvelope.SendingDomainIDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	type key struct{ domain, integration int64 }
	read := make(map[key]*ent.SendingDomainEnvelope, len(rows))
	for _, r := range rows {
		read[key{r.SendingDomainID, r.IntegrationID}] = r
	}
	for _, d := range doms {
		envs := make([]Envelope, 0, len(integs))
		for _, integ := range integs {
			envs = append(envs, envelopeOf(integ, read[key{d.ID, integ.ID}]))
		}
		out[d.ID] = envs
	}
	return out, nil
}

// envelopeOf is the envelope of integ as row, the last read of it for the domain (nil when none
// was made), says.
func envelopeOf(integ *ent.Integration, row *ent.SendingDomainEnvelope) Envelope {
	env := Envelope{IntegrationID: integ.ID, IntegrationName: integ.Name, Provider: integ.Provider}
	switch {
	case integ.Provider != providers.SES:
		env.State = EnvelopeSetAtProvider
	case row == nil:
		env.State, env.PermissionNeeded = EnvelopeUnknown, MailFromPermission
	default:
		env.State, env.CheckedAt = EnvelopeState(row.State), &row.CheckedAt
		switch env.State {
		case EnvelopeCustom:
			env.MailFrom = &messaging.MailFrom{
				Domain: *row.MailFromDomain, Status: *row.MailFromStatus, OnMXFailure: *row.MailFromOnMxFailure,
			}
		case EnvelopeUnknown:
			env.PermissionNeeded = MailFromPermission
		}
	}
	return env
}

// checkEnvelopes reads the envelope of the kept domain at each email Integration whose provider
// keeps one (SES's custom MAIL FROM, ADR 0184) and stores what it found per Integration. Like the
// DMARC lookup it never fails the check nor changes its DKIM result: a read without an answer is
// stored as unknown, and a failure to list or store is logged.
func (m *Module) checkEnvelopes(ctx context.Context, s *ent.Scoped, dom *ent.SendingDomain) {
	integs, err := m.integrations.EnvelopeIntegrations(ctx, s)
	if err != nil {
		slog.WarnContext(ctx, "sending domain envelopes not read",
			"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "err", err)
		return
	}
	for _, integ := range integs {
		mailFrom, keeps, err := m.integrations.ReadMailFrom(ctx, integ, dom.Domain)
		if !keeps {
			continue
		}
		state := EnvelopeProviderDefault
		switch {
		case err != nil:
			slog.WarnContext(ctx, "sending domain envelope unknown",
				"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "integration_id", integ.ID, "err", err)
			state, mailFrom = EnvelopeUnknown, messaging.MailFrom{}
		case mailFrom.Domain != "":
			state = EnvelopeCustom
		}
		if err := recordEnvelope(ctx, s, dom.ID, integ.ID, state, mailFrom, m.now()); err != nil {
			slog.WarnContext(ctx, "sending domain envelope not stored",
				"sending_domain_id", dom.ID, "workspace_id", dom.WorkspaceID, "integration_id", integ.ID, "err", err)
		}
	}
}

// recordEnvelope stores what a read of the domain's envelope at the Integration found, replacing
// the last read whole: a custom MAIL FROM's columns are cleared by a read that found none.
func recordEnvelope(ctx context.Context, s *ent.Scoped, domainID, integrationID int64, state EnvelopeState, mailFrom messaging.MailFrom, now time.Time) error {
	create := s.SendingDomainEnvelope().Create().
		SetSendingDomainID(domainID).
		SetIntegrationID(integrationID).
		SetState(sendingdomainenvelope.State(state)).
		SetCheckedAt(now)
	if state == EnvelopeCustom {
		create.SetMailFromDomain(mailFrom.Domain).
			SetMailFromStatus(mailFrom.Status).
			SetMailFromOnMxFailure(mailFrom.OnMXFailure)
	}
	return create.
		OnConflictColumns(sendingdomainenvelope.FieldWorkspaceID, sendingdomainenvelope.FieldSendingDomainID, sendingdomainenvelope.FieldIntegrationID).
		Update(func(u *ent.SendingDomainEnvelopeScopedUpsert) {
			u.UpdateState().
				UpdateMailFromDomain().
				UpdateMailFromStatus().
				UpdateMailFromOnMxFailure().
				UpdateCheckedAt().
				UpdateUpdatedAt()
		}).
		Exec(ctx)
}
