// Package webhooks is the one place a Workspace's Webhook endpoints are created,
// updated, read, listed and deleted. It owns the URL rule, the signing secret
// (generated server-side, stored sealed, returned decrypted) and the Enterprise
// rule that selecting audit.entry needs a license (ADR 0022). The site and external
// handlers only map DTOs and the sentinels below to a 422. The module publishes no
// events: the endpoint is an audited entity, so the scoped client records the change.
//
// Order: a catalogue, ascending by id (see package pagination).
package webhooks

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/url"
	"slices"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/webhookendpoint"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// Domain errors. Callers match with errors.Is.
var (
	// ErrInvalidURL: the URL is not an absolute http(s) URL (a 422 on `url`).
	ErrInvalidURL = errors.New("webhooks: url must be an absolute http or https URL")
	// ErrAuditNeedsLicense: audit.entry was selected without an Enterprise license
	// (a 422 on `eventTypes`).
	ErrAuditNeedsLicense = errors.New("webhooks: audit.entry needs an Enterprise license")
)

// Licensing reports whether the Enterprise license is active. A nil Licensing
// means unlicensed.
type Licensing interface{ Licensed() bool }

// Endpoint is a Webhook endpoint with its signing secret decrypted.
type Endpoint struct {
	*ent.WebhookEndpoint
	Secret string
}

// CreateInput is a new endpoint. A nil Enabled keeps the default (enabled).
type CreateInput struct {
	URL        string
	EventTypes []string
	Enabled    *bool
}

// UpdateInput changes only what is set: a nil field is left alone.
type UpdateInput struct {
	URL        *string
	EventTypes []string
	Enabled    *bool
}

// Module is the webhooks module. Every call receives the Workspace-scoped client.
type Module struct {
	cipher *secrets.Cipher
	lic    Licensing
}

// New builds the module. lic may be nil (unlicensed).
func New(cipher *secrets.Cipher, lic Licensing) *Module {
	return &Module{cipher: cipher, lic: lic}
}

func (m *Module) open(e *ent.WebhookEndpoint) (Endpoint, error) {
	secret, err := m.cipher.Decrypt(e.SecretEncrypted)
	if err != nil {
		return Endpoint{}, err
	}
	return Endpoint{WebhookEndpoint: e, Secret: string(secret)}, nil
}

func (m *Module) checkTypes(types []string) error {
	if slices.Contains(types, events.NameAuditEntry) && (m.lic == nil || !m.lic.Licensed()) {
		return ErrAuditNeedsLicense
	}
	return nil
}

// Create validates the URL and the event filter, generates the signing secret and
// stores it sealed.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (Endpoint, error) {
	if !validURL(in.URL) {
		return Endpoint{}, ErrInvalidURL
	}
	if err := m.checkTypes(in.EventTypes); err != nil {
		return Endpoint{}, err
	}
	secret, err := generateSecret()
	if err != nil {
		return Endpoint{}, err
	}
	sealed, err := m.cipher.Encrypt([]byte(secret))
	if err != nil {
		return Endpoint{}, err
	}
	e, err := s.WebhookEndpoint().Create().
		SetURL(in.URL).
		SetSecretEncrypted(sealed).
		SetEventTypes(in.EventTypes).
		SetNillableEnabled(in.Enabled).
		Save(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	return m.open(e)
}

// Update applies the set fields. An unknown id is ent.IsNotFound.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (Endpoint, error) {
	if in.URL != nil && !validURL(*in.URL) {
		return Endpoint{}, ErrInvalidURL
	}
	if err := m.checkTypes(in.EventTypes); err != nil {
		return Endpoint{}, err
	}
	upd := s.WebhookEndpoint().UpdateOneID(id).
		SetNillableURL(in.URL).
		SetNillableEnabled(in.Enabled)
	if in.EventTypes != nil {
		upd = upd.SetEventTypes(in.EventTypes)
	}
	if in.Enabled != nil {
		// Whoever flips the switch decides now: the reason it last turned itself off no
		// longer applies, and enabling starts the failure run over (ADR 0039).
		upd = upd.ClearDisabledReason()
		if *in.Enabled {
			upd = upd.ClearConsecutiveFailures().ClearFailingSince().ClearLastErrorKind().
				ClearLastErrorStatus().ClearLastFailureAt()
		}
	}
	e, err := upd.Save(ctx)
	if err != nil {
		return Endpoint{}, err
	}
	return m.open(e)
}

// Delete removes the endpoint. An unknown id is ent.IsNotFound.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return s.WebhookEndpoint().DeleteOneID(id).Exec(ctx)
}

// Get returns one endpoint. An unknown id is ent.IsNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (Endpoint, error) {
	e, err := s.WebhookEndpoint().Get(ctx, id)
	if err != nil {
		return Endpoint{}, err
	}
	return m.open(e)
}

// List returns one page of the Workspace's endpoints, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[Endpoint], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.WebhookEndpoint().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]Endpoint, error) {
			rows, err := s.WebhookEndpoint().Query().
				Order(ent.Asc(webhookendpoint.FieldID)).Limit(limit).Offset(offset).All(ctx)
			if err != nil {
				return nil, err
			}
			out := make([]Endpoint, len(rows))
			for i, e := range rows {
				if out[i], err = m.open(e); err != nil {
					return nil, err
				}
			}
			return out, nil
		})
}

// generateSecret returns a Standard Webhooks signing secret: the "whsec_" prefix
// plus base64 random bytes, the format the standard-webhooks library expects.
func generateSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + base64.StdEncoding.EncodeToString(b), nil
}

// validURL accepts only absolute http(s) URLs. (Network-level SSRF defenses live
// in the delivery worker, which dials the resolved IP.)
func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
