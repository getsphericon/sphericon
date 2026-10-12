// Package integrations is the use-case module for a Workspace's sending-provider
// Integrations: config validation, credential sealing, the default-per-channel rule and
// the redacted read view. /site and /api are thin adapters over it, so the two surfaces
// cannot diverge on how a credential is stored or what is refused.
//
// Handlers hand in a *ent.Scoped and domain-typed input (not their DTOs) and get back
// the row or a domain error: ValidationError (a 422), ErrDefaultConflict (a 409), or
// ent's not-found for an unknown id.
package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/secrets"
)

// ErrDefaultConflict means another default Integration already exists for the channel.
var ErrDefaultConflict = errors.New("a default provider already exists for this channel")

var errUnknownProvider = errors.New("unsupported provider")

// ValidationError is a refused input: Field names the request field, Message is what to
// show for it.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// QuotaEnqueuer schedules a read of one Integration's provider send quota (ADR 0023).
type QuotaEnqueuer interface {
	EnqueueIntegrationQuotaRefresh(ctx context.Context, integrationID int64) error
}

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	bus     *events.Bus
	cipher  *secrets.Cipher
	catalog *messaging.Catalog
	quota   QuotaEnqueuer
}

func New(bus *events.Bus, cipher *secrets.Cipher, catalog *messaging.Catalog, quota QuotaEnqueuer) *Module {
	return &Module{bus: bus, cipher: cipher, catalog: catalog, quota: quota}
}

// SMTPConfig and SESConfig are the write-side provider settings, secrets included.
type (
	SMTPConfig struct {
		Host     string
		Port     int
		Username string
		Password string
		From     string
		FromName string
	}
	SESConfig struct {
		Region          string
		AccessKeyID     string
		SecretAccessKey string
		From            string
		FromName        string
		Endpoint        string
	}
)

// ConfigInput carries exactly one provider's settings.
type ConfigInput struct {
	SMTP *SMTPConfig
	SES  *SESConfig
}

// Limit is a Send rate limit in a request: unset keeps the stored value (update) or
// means no limit (create); Value nil with Set true clears it.
type Limit struct {
	Set   bool
	Value *int
}

// LimitOf reads a Send rate limit from an optional-nullable request field (the generated
// OptNil types of either API): omitted is unset, null clears, a number sets.
func LimitOf[T ~int32](in interface {
	Get() (T, bool)
	IsSet() bool
}) Limit {
	v, ok := in.Get()
	if !ok {
		return Limit{Set: in.IsSet()}
	}
	n := int(v)
	return Limit{Set: true, Value: &n}
}

// CreateInput is a new Integration. Enabled defaults to true when nil.
type CreateInput struct {
	Name         string
	Enabled      *bool
	IsDefault    bool
	MaxPerSecond Limit
	MaxPerDay    Limit
	Config       ConfigInput
}

// UpdateInput is a partial edit; nil fields keep the stored value. A nil Config keeps the
// credentials; inside a supplied Config, blank secrets keep the stored secret.
type UpdateInput struct {
	Name         *string
	Enabled      *bool
	IsDefault    *bool
	MaxPerSecond Limit
	MaxPerDay    Limit
	Config       *ConfigInput
}

// Create validates and stores an Integration, sealing its credentials.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (*ent.Integration, error) {
	name, err := validName(in.Name)
	if err != nil {
		return nil, err
	}
	provider, channel, plaintext, err := m.encode(in.Config)
	if err != nil {
		return nil, err
	}
	sealed, err := m.cipher.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	var row *ent.Integration
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		// A new default clears the channel's current one first, in the same transaction,
		// so the partial unique index never trips on our own writes.
		if in.IsDefault {
			if err := clearDefault(ctx, ts, channel); err != nil {
				return err
			}
		}
		var err error
		row, err = ts.Integration().Create().
			SetName(name).
			SetChannel(channel).
			SetProvider(provider).
			SetConfigEncrypted(sealed).
			SetEnabled(enabled).
			SetIsDefault(in.IsDefault).
			SetNillableMaxPerSecond(in.MaxPerSecond.Value).
			SetNillableMaxPerDay(in.MaxPerDay.Value).
			Save(ctx)
		return err
	})
	if db.IsUniqueViolation(err) {
		return nil, ErrDefaultConflict
	}
	if err != nil {
		return nil, err
	}
	m.discoverQuota(ctx, row)
	return row, nil
}

// Update applies a partial edit. An unknown id is ent's not-found error.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Integration, error) {
	row, err := s.Integration().Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Validate and collect the changes up front so a refusal short-circuits before the
	// transaction opens.
	var name *string
	if in.Name != nil {
		n, err := validName(*in.Name)
		if err != nil {
			return nil, err
		}
		name = &n
	}
	var sealed *string
	if in.Config != nil {
		provider, _, plaintext, err := m.encode(*in.Config)
		if err != nil {
			return nil, err
		}
		if provider.String() != row.Provider.String() {
			return nil, &ValidationError{Field: "config", Message: i18n.T("errors.integration_kind_mismatch", nil)}
		}
		// Secrets are redacted on read, so a partial edit re-sends blank secret fields:
		// carry the stored secrets forward instead of overwriting them with nothing.
		prev, err := m.cipher.Decrypt(row.ConfigEncrypted)
		if err != nil {
			return nil, err
		}
		if plaintext, err = mergeSecrets(provider, plaintext, prev); err != nil {
			return nil, err
		}
		enc, err := m.cipher.Encrypt(plaintext)
		if err != nil {
			return nil, err
		}
		sealed = &enc
	}

	// Promoting to default touches sibling rows, so it clears the channel's current
	// default first, in the same transaction as the update.
	promote := in.IsDefault != nil && *in.IsDefault && !row.IsDefault

	var updated *ent.Integration
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		if promote {
			if err := clearDefault(ctx, ts, row.Channel); err != nil {
				return err
			}
		}
		upd := ts.Integration().UpdateOneID(row.ID)
		if name != nil {
			upd.SetName(*name)
		}
		if in.Enabled != nil {
			upd.SetEnabled(*in.Enabled)
		}
		if sealed != nil {
			upd.SetConfigEncrypted(*sealed)
		}
		applyLimit(in.MaxPerSecond, upd.SetMaxPerSecond, upd.ClearMaxPerSecond)
		applyLimit(in.MaxPerDay, upd.SetMaxPerDay, upd.ClearMaxPerDay)
		if in.IsDefault != nil {
			upd.SetIsDefault(*in.IsDefault)
		}
		var err error
		updated, err = upd.Save(ctx)
		return err
	})
	if db.IsUniqueViolation(err) {
		return nil, ErrDefaultConflict
	}
	if err != nil {
		return nil, err
	}
	if sealed != nil {
		// New credentials or endpoint: what the provider allows may have changed.
		m.discoverQuota(ctx, updated)
	}
	return updated, nil
}

// Delete removes an Integration. An unknown id is ent's not-found error.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	return m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		return ts.Integration().DeleteOneID(id).Exec(ctx)
	})
}

func applyLimit[T any](l Limit, set func(int) T, clear func() T) {
	switch {
	case l.Value != nil:
		set(*l.Value)
	case l.Set:
		clear()
	}
}

func validName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		msg := i18n.T("errors.name_empty", nil)
		return "", &ValidationError{Field: "name", Message: msg}
	}
	return name, nil
}

// discoverQuota schedules a saved SES Integration's send quota lookup (ADR 0023).
// Best-effort: a failure is logged, never returned, so it cannot fail the save (the
// hourly job picks the Integration up anyway). Providers without a quota are skipped.
func (m *Module) discoverQuota(ctx context.Context, row *ent.Integration) {
	if row.Provider != integration.ProviderSes || m.quota == nil {
		return
	}
	if err := m.quota.EnqueueIntegrationQuotaRefresh(ctx, row.ID); err != nil {
		slog.WarnContext(ctx, "enqueue integration quota refresh failed", "integration_id", row.ID, "err", err)
	}
}

// clearDefault unsets the existing default Integration for the channel.
func clearDefault(ctx context.Context, ts *ent.Scoped, channel integration.Channel) error {
	_, err := ts.Integration().Update().
		Where(integration.ChannelEQ(channel), integration.IsDefault(true)).
		SetIsDefault(false).
		Save(ctx)
	return err
}

// encode validates the provider config, derives the provider and channel, and returns
// the cleartext JSON to seal.
func (m *Module) encode(in ConfigInput) (integration.Provider, integration.Channel, []byte, error) {
	var (
		provider  integration.Provider
		plaintext []byte
		err       error
	)
	switch {
	case in.SMTP != nil:
		c := in.SMTP
		provider = integration.ProviderSMTP
		plaintext, err = json.Marshal(smtp.Config{
			Host: c.Host, Port: c.Port, Username: c.Username, Password: c.Password, From: c.From, FromName: c.FromName,
		})
	case in.SES != nil:
		c := in.SES
		provider = integration.ProviderSes
		plaintext, err = json.Marshal(ses.Config{
			Region: c.Region, AccessKeyID: c.AccessKeyID, SecretAccessKey: c.SecretAccessKey,
			From: c.From, FromName: c.FromName, Endpoint: c.Endpoint,
		})
	default:
		return "", "", nil, configError(errUnknownProvider)
	}
	if err != nil {
		return "", "", nil, err
	}

	channel, ok := m.catalog.ChannelOf(messaging.Provider(provider))
	if !ok {
		return "", "", nil, configError(errUnknownProvider)
	}
	if err := m.catalog.Validate(channel, messaging.Provider(provider), plaintext); err != nil {
		return "", "", nil, configError(err)
	}
	return provider, integration.Channel(channel), plaintext, nil
}

func configError(err error) *ValidationError {
	return &ValidationError{Field: "config", Message: err.Error()}
}

// mergeSecrets carries forward stored secrets that the client did not resupply. Secrets
// are redacted on read, so a config update re-sends blank secret fields; an empty secret
// therefore means "keep the stored value", not "clear it". The provider kind is
// guaranteed to match (checked before this is called).
func mergeSecrets(provider integration.Provider, next, prev []byte) ([]byte, error) {
	switch provider {
	case integration.ProviderSMTP:
		var n, p smtp.Config
		if err := json.Unmarshal(next, &n); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(prev, &p); err != nil {
			return nil, err
		}
		if n.Password == "" {
			n.Password = p.Password
		}
		return json.Marshal(n)
	case integration.ProviderSes:
		var n, p ses.Config
		if err := json.Unmarshal(next, &n); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(prev, &p); err != nil {
			return nil, err
		}
		if n.SecretAccessKey == "" {
			n.SecretAccessKey = p.SecretAccessKey
		}
		return json.Marshal(n)
	}
	return next, nil
}

// RedactedConfig is the read-side provider config: no password, no secret key.
type RedactedConfig struct {
	SMTP *RedactedSMTP
	SES  *RedactedSES
}

type (
	RedactedSMTP struct {
		Host     string
		Port     int
		Username string
		From     string
		FromName string
	}
	RedactedSES struct {
		Region           string
		From             string
		FromName         string
		Endpoint         string
		AccessKeyIDLast4 string
	}
)

// Redact opens the stored config and returns it without secrets.
func (m *Module) Redact(row *ent.Integration) (RedactedConfig, error) {
	plaintext, err := m.cipher.Decrypt(row.ConfigEncrypted)
	if err != nil {
		return RedactedConfig{}, err
	}
	switch row.Provider {
	case integration.ProviderSMTP:
		var c smtp.Config
		if err := json.Unmarshal(plaintext, &c); err != nil {
			return RedactedConfig{}, err
		}
		return RedactedConfig{SMTP: &RedactedSMTP{
			Host: c.Host, Port: c.Port, Username: c.Username, From: c.From, FromName: c.FromName,
		}}, nil
	case integration.ProviderSes:
		var c ses.Config
		if err := json.Unmarshal(plaintext, &c); err != nil {
			return RedactedConfig{}, err
		}
		last4 := c.AccessKeyID
		if n := len(last4); n > 4 {
			last4 = last4[n-4:]
		}
		return RedactedConfig{SES: &RedactedSES{
			Region: c.Region, From: c.From, FromName: c.FromName, Endpoint: c.Endpoint, AccessKeyIDLast4: last4,
		}}, nil
	}
	return RedactedConfig{}, errUnknownProvider
}

// List returns one page of the Workspace's Integrations, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.Integration], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.Integration().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Integration, error) {
			return s.Integration().Query().Order(ent.Asc(integration.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}
