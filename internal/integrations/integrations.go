// Package integrations is the use-case module for a Workspace's sending-provider
// Integrations: config validation, credential sealing, the default-per-channel rule and
// the redacted read view. /site and /api are thin adapters over it, so the two surfaces
// cannot diverge on how a credential is stored or what is refused.
//
// Handlers hand in a *ent.Scoped and domain-typed input (not their DTOs) and get back
// the row or a domain error: validation.Error (a 422), ErrDefaultConflict (a 409), or
// ent's not-found for an unknown id.
package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/automationversion"
	"github.com/getsphericon/sphericon/ent/broadcast"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/credentials"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/i18n"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/liveuse"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/pagination"
	"github.com/getsphericon/sphericon/internal/preset"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/validation"
)

// ErrDefaultConflict means another default Integration already exists for the channel.
var ErrDefaultConflict = validation.Conflict("a default provider already exists for this channel", "errors.integration_default_conflict")

// ErrNotFound means there is no such Integration in this Workspace.
var ErrNotFound = validation.NotFound("integrations: integration not found", "errors.integration_not_found")

// ErrOneInbox means the Workspace already keeps a Test inbox: it has at most one (ADR 0058).
var ErrOneInbox = validation.Conflict("a workspace has at most one Test inbox", "errors.integration_one_inbox")

// oneInboxIndex is the partial unique index that keeps at most one kept Test inbox per Workspace.
const oneInboxIndex = "integrations_one_inbox_per_workspace"

// conflictOf names which rule a unique violation broke: the one Test inbox per Workspace, else
// the one default per channel. Any other error is returned as it is.
func conflictOf(err error) error {
	if !db.IsUniqueViolation(err) {
		return err
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == oneInboxIndex {
		return ErrOneInbox
	}
	return ErrDefaultConflict
}

var errUnknownProvider = errors.New("unsupported provider")

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
	presets *preset.Registry
	appURL  string
	confirm func(ctx context.Context, url string) error
}

// missing turns the store's miss into ErrNotFound.
func missing(err error) error {
	if ent.IsNotFound(err) {
		return ErrNotFound
	}
	return err
}

// Get returns one Integration of the Workspace. An unknown id is ErrNotFound.
func (m *Module) Get(ctx context.Context, s *ent.Scoped, id int64) (*ent.Integration, error) {
	row, err := s.Integration().Get(ctx, id)
	if err != nil {
		return nil, missing(err)
	}
	return row, nil
}

// Option tunes the module.
type Option func(*Module)

// WithSubscriptionConfirmer replaces how a Preset's confirm action is performed: a GET
// through the SSRF-guarded client by default.
func WithSubscriptionConfirmer(f func(ctx context.Context, url string) error) Option {
	return func(m *Module) { m.confirm = f }
}

// New builds the module. presets is the Instance's Preset registry and appURL the public
// origin the hook URLs are built from.
func New(bus *events.Bus, cipher *secrets.Cipher, catalog *messaging.Catalog, quota QuotaEnqueuer, presets *preset.Registry, appURL string, opts ...Option) *Module {
	m := &Module{bus: bus, cipher: cipher, catalog: catalog, quota: quota, presets: presets, appURL: appURL, confirm: confirmByGet}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// SMTPConfig and SESConfig are the write-side provider settings, secrets included.
type (
	// InboxConfig is the Test inbox's settings: it has none.
	InboxConfig struct{}

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
	SMTP  *SMTPConfig
	SES   *SESConfig
	Inbox *InboxConfig
}

// Limit is a Provider limit in a request: unset keeps the stored value (update) or
// means no limit (create); Value nil with Set true clears it.
type Limit struct {
	Set   bool
	Value *int
}

// LimitOf reads a Provider limit from an optional-nullable request field (the generated
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
	// Preset and Params are the inbound side (ADR 0037); Params without a Preset is refused.
	Preset *string
	Params map[string]any
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
	// Preset, Params and RetireSecrets edit the inbound side; see sealInbound.
	Preset        PresetChange
	Params        map[string]any
	RetireSecrets bool
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
	if in.Config.SES != nil {
		if err := requireProviderLimit(in.Config.SES.Endpoint, in.MaxPerSecond.Value, in.MaxPerDay.Value); err != nil {
			return nil, err
		}
	}
	sealed, err := m.cipher.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	// An SES Integration reads its callbacks with the `ses` Preset unless told otherwise.
	presetName := in.Preset
	if presetName == nil && provider == integration.ProviderSes {
		presetName = new(string)
		*presetName = "ses"
	}
	inb, err := m.sealInbound(nil, inboundChange{preset: PresetChange{Set: presetName != nil, Name: presetName}, params: in.Params})
	if err != nil {
		return nil, err
	}
	key, err := credentials.GenerateIngestKey()
	if err != nil {
		return nil, err
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
		create := ts.Integration().Create().
			SetName(name).
			SetChannel(channel).
			SetProvider(provider).
			SetConfigEncrypted(sealed).
			SetIngestKey(key).
			SetNillablePreset(inb.preset).
			SetEnabled(enabled).
			SetIsDefault(in.IsDefault).
			SetNillableMaxPerSecond(in.MaxPerSecond.Value).
			SetNillableMaxPerDay(in.MaxPerDay.Value)
		if inb.sealed != nil {
			create.SetParamsEncrypted(*inb.sealed)
		}
		row, err = create.Save(ctx)
		return err
	})
	if err != nil {
		return nil, conflictOf(err)
	}
	m.discoverQuota(ctx, row)
	return row, nil
}

// Update applies a partial edit. An unknown id is ErrNotFound.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Integration, error) {
	row, err := m.Get(ctx, s, id)
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
	sealed, err := m.sealEditedConfig(row, in.Config)
	if err != nil {
		return nil, err
	}
	if err := m.keepsProviderLimit(row, in); err != nil {
		return nil, err
	}

	inb, err := m.sealInbound(row, inboundChange{preset: in.Preset, params: in.Params, retireSecrets: in.RetireSecrets})
	if err != nil {
		return nil, err
	}

	// Promoting to default touches sibling rows, so it clears the channel's current
	// default first, in the same transaction as the update.
	promote := in.IsDefault != nil && *in.IsDefault && !row.IsDefault
	if promote && row.DiscardedAt != nil {
		return nil, &validation.Error{Field: "isDefault", Message: i18n.T("errors.integration_archived_default", nil)}
	}

	var updated *ent.Integration
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		if promote {
			if err := clearDefault(ctx, ts, row.Channel); err != nil {
				return err
			}
		}
		upd := ts.Integration().UpdateOneID(row.ID)
		applyEdit(upd, in, name, sealed, inb)
		var err error
		updated, err = upd.Save(ctx)
		return err
	})
	if err != nil {
		return nil, conflictOf(err)
	}
	if sealed != nil {
		// New credentials or endpoint: what the provider allows may have changed.
		m.discoverQuota(ctx, updated)
	}
	return updated, nil
}

// sealEditedConfig validates an edited provider config and seals it, nil when the edit sends
// none. Secrets are redacted on read, so a partial edit re-sends blank secret fields: the stored
// secrets are carried forward instead of being overwritten with nothing.
func (m *Module) sealEditedConfig(row *ent.Integration, cfg *ConfigInput) (*string, error) {
	if cfg == nil {
		return nil, nil
	}
	provider, _, plaintext, err := m.encode(*cfg)
	if err != nil {
		return nil, err
	}
	if provider.String() != row.Provider.String() {
		return nil, &validation.Error{Field: "config", Message: i18n.T("errors.integration_kind_mismatch", nil)}
	}
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
	return &enc, nil
}

// applyEdit sets on upd what an Update changes: the validated name, the sealed config, the inbound
// side and the plain settings the edit names.
func applyEdit(upd *ent.IntegrationScopedUpdateOne, in UpdateInput, name, sealed *string, inb inbound) {
	if name != nil {
		upd.SetName(*name)
	}
	if in.Enabled != nil {
		upd.SetEnabled(*in.Enabled)
	}
	if sealed != nil {
		upd.SetConfigEncrypted(*sealed)
	}
	if inb.apply {
		if inb.preset == nil {
			upd.ClearPreset().ClearParamsEncrypted()
		} else {
			upd.SetPreset(*inb.preset).SetParamsEncrypted(*inb.sealed)
		}
	}
	applyLimit(in.MaxPerSecond, upd.SetMaxPerSecond, upd.ClearMaxPerSecond)
	applyLimit(in.MaxPerDay, upd.SetMaxPerDay, upd.ClearMaxPerDay)
	if in.IsDefault != nil {
		upd.SetIsDefault(*in.IsDefault)
	}
}

// Archive discards an Integration (ADR 0049), unless live work uses it: then it returns an
// *inuse.Error naming that work. A scheduled or sending Broadcast uses the Integration it names,
// and the default one when it names none; an active Automation that sends email uses the one it
// names, and the default one when it names none. A sent or failed Broadcast is history and does not
// block. An archived Integration gives up the default: its channel has none until another is
// picked. Archiving an archived Integration changes nothing. An unknown id is ErrNotFound.
func (m *Module) Archive(ctx context.Context, s *ent.Scoped, id int64) error {
	return missing(m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := ts.Integration().Get(ctx, id)
		if err != nil {
			return err
		}
		if row.DiscardedAt != nil {
			return nil
		}
		users, err := liveUsers(ctx, ts, row)
		if err != nil {
			return err
		}
		if len(users) > 0 {
			return &inuse.Error{What: inuse.SubjectIntegration, Users: users}
		}
		if row.IsDefault {
			if err := ts.Integration().UpdateOneID(id).SetIsDefault(false).Exec(ctx); err != nil {
				return err
			}
		}
		return ts.Integration().Discard(ctx, id)
	}))
}

// Unarchive brings an archived Integration back to the lists. It does not become the default
// again. Restoring an Integration that is not archived changes nothing. An unknown id is
// ErrNotFound.
func (m *Module) Unarchive(ctx context.Context, s *ent.Scoped, id int64) error {
	return conflictOf(missing(s.Integration().Restore(ctx, id)))
}

// liveUsers is the live work that uses the Integration.
func liveUsers(ctx context.Context, s *ent.Scoped, row *ent.Integration) ([]inuse.User, error) {
	uses := broadcast.IntegrationID(row.ID)
	if row.IsDefault {
		uses = broadcast.Or(uses, broadcast.IntegrationIDIsNil())
	}
	live, err := s.Broadcast().Query().
		Where(uses, broadcast.StatusIn(broadcast.StatusScheduled, broadcast.StatusSending)).
		Order(ent.Asc(broadcast.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	users := inuse.Broadcasts(live)
	if row.Channel != integration.ChannelEmail {
		return users, nil
	}
	sendsThrough := automationversion.IntegrationID(row.ID)
	if row.IsDefault {
		sendsThrough = automationversion.Or(sendsThrough, automationversion.IntegrationIDIsNil())
	}
	sending, err := liveuse.EmailAutomations(ctx, s, sendsThrough)
	if err != nil {
		return nil, err
	}
	return append(users, sending...), nil
}

func applyLimit[T any](l Limit, set func(int) T, unset func() T) {
	switch {
	case l.Value != nil:
		set(*l.Value)
	case l.Set:
		unset()
	}
}

// requireProviderLimit refuses an SES Integration with its own endpoint that is left without
// either Provider limit value (ADR 0067): an SES-compatible service has no quota call, so the
// customer gives what their provider account allows. AWS SES (no endpoint) reports its own.
func requireProviderLimit(endpoint string, perSecond, perDay *int) error {
	if endpoint == "" {
		return nil
	}
	msg := i18n.T("errors.integration_provider_limit_required", nil)
	missing := map[string][]string{}
	if perSecond == nil {
		missing[validation.FieldMaxPerSecond] = []string{msg}
	}
	if perDay == nil {
		missing[validation.FieldMaxPerDay] = []string{msg}
	}
	if len(missing) == 0 {
		return nil
	}
	return &validation.FieldsError{Fields: missing}
}

// keepsProviderLimit applies requireProviderLimit to what an Update leaves: the edited SES
// config's endpoint or the stored one, and each limit as edited or as stored.
func (m *Module) keepsProviderLimit(row *ent.Integration, in UpdateInput) error {
	if row.Provider != integration.ProviderSes {
		return nil
	}
	var endpoint string
	if in.Config != nil && in.Config.SES != nil {
		endpoint = in.Config.SES.Endpoint
	} else {
		stored, err := m.Redact(row)
		if err != nil {
			return err
		}
		endpoint = stored.SES.Endpoint
	}
	return requireProviderLimit(endpoint, in.MaxPerSecond.after(row.MaxPerSecond), in.MaxPerDay.after(row.MaxPerDay))
}

// after is the limit an edit leaves: the request's when it names one, else the stored one.
func (l Limit) after(stored *int) *int {
	if l.Set {
		return l.Value
	}
	return stored
}

func validName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		msg := i18n.T("errors.name_empty", nil)
		return "", &validation.Error{Field: "name", Message: msg}
	}
	return name, nil
}

// discoverQuota schedules a saved Integration's send quota lookup (ADR 0023), when its provider
// can report one (SES; the Test inbox its own ceiling).
// Best-effort: a failure is logged, never returned, so it cannot fail the save (the
// hourly job picks an SES Integration up anyway). Providers without a quota are skipped.
func (m *Module) discoverQuota(ctx context.Context, row *ent.Integration) {
	if m.quota == nil || !m.reportsQuota(row) {
		return
	}
	if err := m.quota.EnqueueIntegrationQuotaRefresh(ctx, row.ID); err != nil {
		slog.WarnContext(ctx, "enqueue integration quota refresh failed", "integration_id", row.ID, "err", err)
	}
}

// reportsQuota reports whether the Integration's provider can say what it allows: whether its
// sender is a messaging.QuotaReader.
func (m *Module) reportsQuota(row *ent.Integration) bool {
	config, err := m.cipher.Decrypt(row.ConfigEncrypted)
	if err != nil {
		return false
	}
	built, err := m.catalog.BuildEmail(messaging.Provider(row.Provider), config, messaging.Deps{})
	if err != nil {
		return false
	}
	_, ok := messaging.AsQuotaReader(built)
	return ok
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
	case in.Inbox != nil:
		provider = integration.ProviderInbox
		plaintext = []byte("{}")
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

func configError(err error) *validation.Error {
	return &validation.Error{Field: "config", Message: err.Error()}
}

// mergeSecrets carries forward a stored secret that the client did not resupply. Secrets
// are redacted on read, so a config update re-sends a blank secret field: blank means
// "keep the stored value", not "clear it". A stored secret is only ever sent to the place
// and as the identity it was saved for, so it is kept only while that destination is
// unchanged (SMTP: host, port, username; SES: region, endpoint, access key id). Pointing
// the Integration elsewhere with a blank secret is a refusal, never a silent carry-over:
// the caller must supply the secret again. The provider kind is guaranteed to match
// (checked before this is called).
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
		if n.Password == "" && p.Password != "" {
			if n.Host != p.Host || n.Port != p.Port || n.Username != p.Username {
				return nil, secretRequired()
			}
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
		if n.SecretAccessKey == "" && p.SecretAccessKey != "" {
			if n.Region != p.Region || n.Endpoint != p.Endpoint || n.AccessKeyID != p.AccessKeyID {
				return nil, secretRequired()
			}
			n.SecretAccessKey = p.SecretAccessKey
		}
		return json.Marshal(n)
	case integration.ProviderInbox:
		return next, nil // no secret
	}
	return next, nil
}

func secretRequired() *validation.Error {
	return &validation.Error{Field: "config", Message: i18n.T("errors.integration_secret_required", nil)}
}

// RedactedConfig is the read-side provider config: no password, no secret key.
type RedactedConfig struct {
	SMTP  *RedactedSMTP
	SES   *RedactedSES
	Inbox *RedactedInbox
}

type (
	// RedactedInbox is the Test inbox's config: it has no field.
	RedactedInbox struct{}

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
	case integration.ProviderInbox:
		return RedactedConfig{Inbox: &RedactedInbox{}}, nil
	}
	return RedactedConfig{}, errUnknownProvider
}

// rows is the Workspace's Integrations: the kept ones, or with archived the archived ones.
func rows(s *ent.Scoped, archived bool) *ent.IntegrationQuery {
	if archived {
		return s.Integration().QueryDiscarded()
	}
	return s.Integration().Query()
}

// All returns every kept Integration of the Workspace, or with archived every archived one,
// ascending by id.
func (m *Module) All(ctx context.Context, s *ent.Scoped, archived bool) ([]*ent.Integration, error) {
	return rows(s, archived).Order(ent.Asc(integration.FieldID)).All(ctx)
}

// List returns one page of the Workspace's Integrations, ascending by id: the kept ones, or with
// archived the archived ones.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params, archived bool) (pagination.Page[*ent.Integration], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return rows(s, archived).Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Integration, error) {
			return rows(s, archived).Order(ent.Asc(integration.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}
