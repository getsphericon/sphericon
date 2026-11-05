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
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/events"
	"github.com/getsphericon/sphericon/internal/inuse"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/pagination"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
	"github.com/getsphericon/sphericon/internal/liveuse"
	"github.com/getsphericon/sphericon/internal/messaging"
	"github.com/getsphericon/sphericon/internal/messaging/ses"
	"github.com/getsphericon/sphericon/internal/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/messaging/telegram"
	"github.com/getsphericon/sphericon/internal/preset"
)

// fieldConfig is the field a refusal of the provider config is on.
const fieldConfig = "config"

// fieldPreset is the field a refusal of the Preset is on.
const fieldPreset = "preset"

// detailKey is the template value that carries a provider's own words into a refusal.
const detailKey = "Detail"

// ErrDefaultConflict means another default Integration already exists for the channel.
var ErrDefaultConflict = validation.Conflict("a default provider already exists for this channel", i18n.ErrorsIntegrationDefaultConflict)

// ErrNotFound means there is no such Integration in this Workspace.
var ErrNotFound = validation.NotFound("integrations: integration not found", i18n.ErrorsIntegrationNotFound)

// ErrOneInbox means the Workspace already keeps a Test inbox: it has at most one (ADR 0058).
var ErrOneInbox = validation.Conflict("a workspace has at most one Test inbox", i18n.ErrorsIntegrationOneInbox)

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

// editable returns the Integration of the id for its owner to change. A deleted one is read, never
// changed (ADR 0100): it is ErrNotFound, as an unknown id is.
func (m *Module) editable(ctx context.Context, s *ent.Scoped, id int64) (*ent.Integration, error) {
	row, err := m.Get(ctx, s, id)
	if err != nil {
		return nil, err
	}
	if row.DiscardedAt != nil {
		return nil, ErrNotFound
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
	SMTP     *SMTPConfig
	SES      *SESConfig
	Inbox    *InboxConfig
	Telegram *TelegramConfig
	Twilio   *TwilioConfig
}

// CreateInput is a new Integration. Enabled defaults to true when nil; a nil Provider limit
// is no limit.
type CreateInput struct {
	Name         string
	Enabled      *bool
	IsDefault    bool
	MaxPerSecond *int
	MaxPerDay    *int
	Config       ConfigInput
	// Preset and Params are the inbound side (ADR 0037); Params without a Preset is refused.
	Preset *string
	Params map[string]any
}

// UpdateInput is a patch of an Integration: a field that keeps leaves the stored value. A Provider
// limit clears to no limit. An absent Config leaves the credentials as they are; inside a set Config,
// blank secrets keep the stored secret. Name, Enabled, IsDefault and Config cannot be cleared.
type UpdateInput struct {
	Name         patch.Field[string]
	Enabled      patch.Field[bool]
	IsDefault    patch.Field[bool]
	MaxPerSecond patch.Field[int]
	MaxPerDay    patch.Field[int]
	Config       patch.Field[ConfigInput]
	// Preset, Params and RetireSecrets edit the inbound side; see sealInbound. A Preset clears to
	// none; Params are merged over the stored ones.
	Preset        patch.Field[string]
	Params        patch.Field[map[string]any]
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
	if err := presetFits(provider, in.Preset, in.Params); err != nil {
		return nil, err
	}
	if in.Config.SES != nil {
		if err := requireProviderLimit(in.Config.SES.Endpoint, in.MaxPerSecond, in.MaxPerDay); err != nil {
			return nil, err
		}
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	// An SES Integration reads its callbacks with the `ses` Preset unless told otherwise; a Telegram
	// one always with the `telegram` Preset, its webhook secret generated here; a Twilio one always
	// with the `twilio` Preset, its Auth Token the caller's param.
	presetName, params := in.Preset, in.Params
	var providerPerSecond *int
	switch provider {
	case providers.SES:
		if presetName == nil {
			presetName = new(string)
			*presetName = "ses"
		}
	case providers.SMTP, providers.Inbox:
		// The Preset the caller named, if any.
	case providers.Telegram:
		presetName, params = new(string), telegramParams()
		*presetName = TelegramPreset
		providerPerSecond = new(int)
		*providerPerSecond = telegramPerSecond
	case providers.Twilio:
		presetName = new(string)
		*presetName = TwilioPreset
	}
	inb, err := m.sealInbound(nil, inboundChange{preset: patch.FromPtr(presetName), params: params})
	if err != nil {
		return nil, err
	}
	key, err := credentials.GenerateIngestKey()
	if err != nil {
		return nil, err
	}
	if provider == providers.Telegram {
		// The bot is connected before the save: Telegram refuses a wrong token or a webhook it cannot
		// reach, and the save is refused with it.
		if plaintext, err = m.connectNew(ctx, tokenOf(in.Config.Telegram), key, currentSecret(params["secret"]), enabled); err != nil {
			return nil, err
		}
	}
	if provider == providers.Twilio {
		// The account is checked and its sender pointed at the hook before the save: Twilio refuses
		// a wrong or revoked key, and the save is refused with it.
		if plaintext, err = m.connectTwilio(ctx, twilioStored(in.Config.Twilio), key); err != nil {
			return nil, err
		}
	}
	sealed, err := m.cipher.Encrypt(plaintext)
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
			SetNillableMaxPerSecond(in.MaxPerSecond).
			SetNillableMaxPerDay(in.MaxPerDay).
			SetNillableProviderMaxPerSecond(providerPerSecond)
		if inb.sealed != nil {
			create.SetParamsEncrypted(*inb.sealed)
		}
		row, err = create.Save(ctx)
		return err
	})
	if err != nil {
		if provider == providers.Telegram && enabled {
			// The bot was pointed at a hook that was never saved.
			m.releaseToken(ctx, 0, tokenOf(in.Config.Telegram))
		}
		return nil, conflictOf(err)
	}
	m.discoverQuota(ctx, row)
	return row, nil
}

// Update applies a partial edit. An unknown id, or a deleted Integration's, is ErrNotFound.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (*ent.Integration, error) {
	row, err := m.editable(ctx, s, id)
	if err != nil {
		return nil, err
	}

	// Validate and collect the changes up front so a refusal short-circuits before the
	// transaction opens.
	var name *string
	if raw, ok := in.Name.Get(); ok {
		n, err := validName(raw)
		if err != nil {
			return nil, err
		}
		name = &n
	}
	var sealed *string
	var tg telegramEdit
	if row.Provider == providers.Telegram {
		// The bot is connected, as the edit asks, before the save commits.
		if tg, err = m.editTelegram(ctx, row, in); err != nil {
			return nil, err
		}
		sealed = tg.sealed
	} else if row.Provider == providers.Twilio {
		// New credentials or a new sender are checked with Twilio before the save commits.
		if sealed, err = m.editTwilio(ctx, row, in); err != nil {
			return nil, err
		}
	} else if sealed, err = m.sealEditedConfig(row, in.Config.Ptr()); err != nil {
		return nil, err
	}
	if err := m.keepsProviderLimit(row, in); err != nil {
		return nil, err
	}

	params, _ := in.Params.Get()
	inb, err := m.sealInbound(row, inboundChange{preset: in.Preset, params: params, retireSecrets: in.RetireSecrets})
	if err != nil {
		return nil, err
	}

	// Promoting to default touches sibling rows, so it clears the channel's current
	// default first, in the same transaction as the update.
	isDefault, _ := in.IsDefault.Get()
	promote := isDefault && !row.IsDefault

	var updated *ent.Integration
	err = m.bus.WithinScopedTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		if promote {
			if err := clearDefault(ctx, ts, row.Channel); err != nil {
				return err
			}
		}
		// A Delete that landed meanwhile leaves no row to edit.
		upd := ts.Integration().UpdateOneID(row.ID).Where(integration.DiscardedAtIsNil())
		applyEdit(upd, in, name, sealed, inb)
		if tg.connected {
			upd.SetProviderMaxPerSecond(telegramPerSecond)
		}
		var err error
		updated, err = upd.Save(ctx)
		return err
	})
	if err != nil {
		return nil, missing(conflictOf(err))
	}
	if tg.release {
		m.release(ctx, updated)
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
	if provider != row.Provider {
		return nil, &validation.Error{Field: fieldConfig, Message: i18n.T(i18n.ErrorsIntegrationKindMismatch, nil)}
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
	patch.ApplySet(in.Enabled, upd.SetEnabled)
	if sealed != nil {
		// The account's standing was read with the old credentials, which may name another
		// account: it goes until the read the save schedules, so a failed one holds nothing. The
		// provider's refusal of the old credentials goes with them: new ones are tried at the next send.
		upd.SetConfigEncrypted(*sealed).ClearProviderEnforcement().ClearProviderRefusedAt().SetProviderSandbox(false)
	}
	if inb.apply {
		if inb.preset == nil {
			upd.ClearPreset().ClearParamsEncrypted()
		} else {
			upd.SetPreset(*inb.preset).SetParamsEncrypted(*inb.sealed)
		}
	}
	patch.Apply(in.MaxPerSecond, upd.SetMaxPerSecond, upd.ClearMaxPerSecond)
	patch.Apply(in.MaxPerDay, upd.SetMaxPerDay, upd.ClearMaxPerDay)
	patch.ApplySet(in.IsDefault, upd.SetIsDefault)
}

// Delete discards an Integration for good (ADR 0049): it leaves the lists and is never restored,
// and what it sent keeps its history. Its Live use (ADR 0099) refuses the delete with an
// *inuse.Error naming that work: a scheduled or sending Broadcast, the live version of an active
// Automation or a version an unfinished Enrollment stands on, whose send goes through it, the
// Integration it names or, naming none, its own Channel's default. A sent or failed Broadcast is
// history and does not block. A deleted Integration gives up the default: its channel has none until another is
// picked. Deleting a deleted Integration changes nothing. An unknown id is ErrNotFound. The check
// and the discard hold the Workspace's lock (ADR 0099), so no work becomes live between them.
func (m *Module) Delete(ctx context.Context, s *ent.Scoped, id int64) error {
	var released *ent.Integration
	err := missing(m.bus.WithinWorkspaceLockTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher) error {
		row, err := ts.Integration().Get(ctx, id)
		if err != nil {
			return err
		}
		if row.DiscardedAt != nil {
			return nil
		}
		if row.Provider == providers.Telegram && holdsWebhook(row.Enabled, nil) {
			released = row
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
	if err != nil {
		return err
	}
	if released != nil {
		m.release(ctx, released)
	}
	return nil
}

// liveUsers is the Live use of the Integration (ADR 0099): the work whose sends go through it, by
// the same resolution a send takes (messaging.Through).
func liveUsers(ctx context.Context, s *ent.Scoped, row *ent.Integration) ([]inuse.User, error) {
	sends, err := liveuse.Sends(ctx, s)
	if err != nil {
		return nil, err
	}
	return liveuse.Users(lo.Filter(sends, func(send liveuse.Send, _ int) bool { return send.Uses(row.ID) })), nil
}

// requireProviderLimit refuses an SES Integration with its own endpoint that is left without
// either Provider limit value (ADR 0067): an SES-compatible service has no quota call, so the
// customer gives what their provider account allows. AWS SES (no endpoint) reports its own.
func requireProviderLimit(endpoint string, perSecond, perDay *int) error {
	if endpoint == "" {
		return nil
	}
	msg := i18n.T(i18n.ErrorsIntegrationProviderLimitRequired, nil)
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
	if row.Provider != providers.SES {
		return nil
	}
	var endpoint string
	if cfg, ok := in.Config.Get(); ok && cfg.SES != nil {
		endpoint = cfg.SES.Endpoint
	} else {
		stored, err := m.Redact(row)
		if err != nil {
			return err
		}
		endpoint = stored.SES.Endpoint
	}
	return requireProviderLimit(endpoint, in.MaxPerSecond.Over(row.MaxPerSecond), in.MaxPerDay.Over(row.MaxPerDay))
}

func validName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		msg := i18n.T(i18n.ErrorsNameEmpty, nil)
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
	if row.Channel != channels.Email {
		return false
	}
	config, err := m.cipher.Decrypt(row.ConfigEncrypted)
	if err != nil {
		return false
	}
	built, err := m.catalog.BuildEmail(row.Provider, config, messaging.Deps{})
	if err != nil {
		return false
	}
	_, ok := messaging.AsQuotaReader(built)
	return ok
}

// clearDefault unsets the existing default Integration for the channel.
func clearDefault(ctx context.Context, ts *ent.Scoped, channel channels.Channel) error {
	_, err := ts.Integration().Update().
		Where(integration.ChannelEQ(channel), integration.IsDefault(true)).
		SetIsDefault(false).
		Save(ctx)
	return err
}

// encode validates the provider config, derives the provider and channel, and returns
// the cleartext JSON to seal.
func (m *Module) encode(in ConfigInput) (providers.Provider, channels.Channel, []byte, error) {
	var (
		provider  providers.Provider
		plaintext []byte
		err       error
	)
	switch {
	case in.SMTP != nil:
		c := in.SMTP
		provider = providers.SMTP
		plaintext, err = json.Marshal(smtp.Config{
			Host: c.Host, Port: c.Port, Username: c.Username, Password: c.Password, From: c.From, FromName: c.FromName,
		})
	case in.SES != nil:
		c := in.SES
		provider = providers.SES
		plaintext, err = json.Marshal(ses.Config{
			Region: c.Region, AccessKeyID: c.AccessKeyID, SecretAccessKey: c.SecretAccessKey,
			From: c.From, FromName: c.FromName, Endpoint: c.Endpoint,
		})
	case in.Inbox != nil:
		provider = providers.Inbox
		plaintext = []byte("{}")
	case in.Twilio != nil:
		// A blank secret is an update that keeps the stored one; the rest of the config is checked
		// when it connects.
		c := twilioStored(in.Twilio)
		if c.Secret == "" {
			c.Secret = "kept"
		}
		if err := validTwilio(c); err != nil {
			return "", "", nil, err
		}
		channel, _ := m.catalog.ChannelOf(providers.Twilio)
		plaintext, err = json.Marshal(twilioStored(in.Twilio))
		return providers.Twilio, channel, plaintext, err
	case in.Telegram != nil:
		// A blank token is an update that keeps the stored one; the bot's id and @username come from
		// getMe when it connects.
		token := tokenOf(in.Telegram)
		if token != "" && !telegram.ValidToken(token) {
			return "", "", nil, tokenFormatError()
		}
		channel, _ := m.catalog.ChannelOf(providers.Telegram)
		plaintext, err = json.Marshal(telegram.Config{Token: token})
		return providers.Telegram, channel, plaintext, err
	default:
		return "", "", nil, configError(errUnknownProvider)
	}
	if err != nil {
		return "", "", nil, err
	}

	channel, ok := m.catalog.ChannelOf(provider)
	if !ok {
		return "", "", nil, configError(errUnknownProvider)
	}
	if err := m.catalog.Validate(channel, provider, plaintext); err != nil {
		return "", "", nil, configError(err)
	}
	return provider, channel, plaintext, nil
}

func configError(err error) *validation.Error {
	return &validation.Error{Field: fieldConfig, Message: err.Error()}
}

// mergeSecrets carries forward a stored secret that the client did not resupply. Secrets
// are redacted on read, so a config update re-sends a blank secret field: blank means
// "keep the stored value", not "clear it". A stored secret is only ever sent to the place
// and as the identity it was saved for, so it is kept only while that destination is
// unchanged (SMTP: host, port, username; SES: region, endpoint, access key id). Pointing
// the Integration elsewhere with a blank secret is a refusal, never a silent carry-over:
// the caller must supply the secret again. The provider kind is guaranteed to match
// (checked before this is called).
func mergeSecrets(provider providers.Provider, next, prev []byte) ([]byte, error) {
	switch provider {
	case providers.SMTP:
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
	case providers.SES:
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
	case providers.Inbox:
		return next, nil // no secret
	case providers.Telegram:
		return next, nil // never merged here: editTelegram carries the token forward
	case providers.Twilio:
		return next, nil // never merged here: editTwilio carries the secret forward
	}
	return next, nil
}

func secretRequired() *validation.Error {
	return &validation.Error{Field: fieldConfig, Message: i18n.T(i18n.ErrorsIntegrationSecretRequired, nil)}
}

// RedactedConfig is the read-side provider config: no password, no secret key.
type RedactedConfig struct {
	SMTP     *RedactedSMTP
	SES      *RedactedSES
	Inbox    *RedactedInbox
	Telegram *RedactedTelegram
	Twilio   *RedactedTwilio
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
	case providers.SMTP:
		var c smtp.Config
		if err := json.Unmarshal(plaintext, &c); err != nil {
			return RedactedConfig{}, err
		}
		return RedactedConfig{SMTP: &RedactedSMTP{
			Host: c.Host, Port: c.Port, Username: c.Username, From: c.From, FromName: c.FromName,
		}}, nil
	case providers.SES:
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
	case providers.Inbox:
		return RedactedConfig{Inbox: &RedactedInbox{}}, nil
	case providers.Telegram:
		var c telegram.Config
		if err := json.Unmarshal(plaintext, &c); err != nil {
			return RedactedConfig{}, err
		}
		return RedactedConfig{Telegram: &RedactedTelegram{BotID: c.BotID, Username: c.Username}}, nil
	case providers.Twilio:
		return redactTwilio(plaintext)
	}
	return RedactedConfig{}, errUnknownProvider
}

// All returns every kept Integration of the Workspace, ascending by id.
func (m *Module) All(ctx context.Context, s *ent.Scoped) ([]*ent.Integration, error) {
	return s.Integration().Query().Order(ent.Asc(integration.FieldID)).All(ctx)
}

// List returns one page of the Workspace's kept Integrations, ascending by id.
func (m *Module) List(ctx context.Context, s *ent.Scoped, p pagination.Params) (pagination.Page[*ent.Integration], error) {
	return pagination.List(ctx, p,
		func(ctx context.Context) (int, error) { return s.Integration().Query().Count(ctx) },
		func(ctx context.Context, limit, offset int) ([]*ent.Integration, error) {
			return s.Integration().Query().Order(ent.Asc(integration.FieldID)).Limit(limit).Offset(offset).All(ctx)
		})
}
