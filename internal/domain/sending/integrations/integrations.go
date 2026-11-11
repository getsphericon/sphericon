// Package integrations is the domain module of a Workspace's sending-provider Integrations:
// config validation, credential sealing, the default-per-channel rule, the provider's send quota
// stored on the row and the View every surface answers with (the redacted config and params, the
// hook URL and the Provider limit). /site and /api only map it, so the two surfaces cannot diverge
// on how a credential is stored, what is refused or what is shown. Deleting one is the use case
// internal/usecase/integrationdeletion, which checks its Live use first (ADR 0102).
//
// Handlers hand in a *ent.Scoped and domain-typed input (not their DTOs) and get back
// the View or a domain error: a validation refusal (a 422), ErrDefaultConflict (a 409), or
// ErrNotFound for an unknown id.
package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/ses"
	"github.com/getsphericon/sphericon/internal/infra/messaging/smtp"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram"
	"github.com/getsphericon/sphericon/internal/kernel/bounds"
	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/jobkind"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// fieldConfig is the field a refusal of the provider config is on.
const fieldConfig = "config"

// fieldPreset is the field a refusal of the Preset is on.
const fieldPreset = "preset"

// detailKey is the template value that carries a provider's own words into a refusal.
const detailKey = "Detail"

// ErrDefaultConflict means another default Integration already exists for the channel.
var ErrDefaultConflict = validation.ConflictField("isDefault", "a default provider already exists for this channel", i18n.ErrorsIntegrationDefaultConflict)

// ErrNotFound means there is no such Integration in this Workspace.
var ErrNotFound = validation.NotFound("integrations: integration not found", i18n.ErrorsIntegrationNotFound)

// ErrOneInbox means the Workspace already keeps a Test inbox: it has at most one (ADR 0058).
var ErrOneInbox = validation.Conflict("a workspace has at most one Test inbox", i18n.ErrorsIntegrationOneInbox)

// The unique constraints of the Integrations (ADR 0106): one default per channel and one kept
// Test inbox per Workspace are refused; the ingest key is random.
var constraints = db.Declare(
	db.Refusal("integration_workspace_id_channel", ErrDefaultConflict),
	db.Refusal("integrations_one_inbox_per_workspace", ErrOneInbox),
	db.Fault("integrations_ingest_key_key", "an ingest key is random (credentials.GenerateIngestKey)"),
)

var errUnknownProvider = errors.New("unsupported provider")

// RefreshQuotaArgs is the job that reads one Integration's provider send quota (ADR 0023); the
// job runner works it (internal/jobs).
type RefreshQuotaArgs struct {
	jobkind.Workspace

	IntegrationID int64 `json:"integration_id"`
}

// Kind is the job's kind in the queue.
func (RefreshQuotaArgs) Kind() string { return "integration_quota_refresh" }

// Module holds the shared collaborators; it is stateless otherwise.
type Module struct {
	bus     *events.Bus
	cipher  *secrets.Cipher
	catalog *messaging.Catalog
	presets *preset.Registry
	appURL  string
	now     func() time.Time
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

// New builds the module. presets is the Instance's Preset registry, appURL the public origin the
// hook URLs are built from and now the Instance clock a quota lookup and the 24-hour usage are
// read against.
func New(bus *events.Bus, cipher *secrets.Cipher, catalog *messaging.Catalog, presets *preset.Registry, appURL string, now func() time.Time, opts ...Option) *Module {
	m := &Module{bus: bus, cipher: cipher, catalog: catalog, presets: presets, appURL: appURL, now: now, confirm: confirmByGet}
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

// Create validates and stores an Integration, sealing its credentials, and answers its View.
func (m *Module) Create(ctx context.Context, s *ent.Scoped, in CreateInput) (View, error) {
	name, err := validation.Name(in.Name)
	if err != nil {
		return View{}, err
	}
	if err := checkProviderLimit(in.MaxPerSecond, in.MaxPerDay); err != nil {
		return View{}, err
	}
	provider, channel, plaintext, err := m.encode(in.Config)
	if err != nil {
		return View{}, err
	}
	if err := presetFits(provider, in.Preset, in.Params); err != nil {
		return View{}, err
	}
	if in.Config.SES != nil {
		if err := requireProviderLimit(in.Config.SES.Endpoint, in.MaxPerSecond, in.MaxPerDay); err != nil {
			return View{}, err
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
		return View{}, err
	}
	key := credentials.GenerateIngestKey()
	if provider == providers.Telegram {
		// The bot is connected before the save: Telegram refuses a wrong token or a webhook it cannot
		// reach, and the save is refused with it.
		if plaintext, err = m.connectNew(ctx, tokenOf(in.Config.Telegram), key, currentSecret(params["secret"]), enabled); err != nil {
			return View{}, err
		}
	}
	if provider == providers.Twilio {
		// The account is checked and its sender pointed at the hook before the save: Twilio refuses
		// a wrong or revoked key, and the save is refused with it.
		if plaintext, err = m.connectTwilio(ctx, twilioStored(in.Config.Twilio), key); err != nil {
			return View{}, err
		}
	}
	sealed, err := m.cipher.Encrypt(plaintext)
	if err != nil {
		return View{}, err
	}

	var row *ent.Integration
	insert := func(ts *ent.Scoped, q events.Enqueuer, isDefault bool) error {
		create := ts.Integration().Create().
			SetName(name).
			SetChannel(channel).
			SetProvider(provider).
			SetConfigEncrypted(sealed).
			SetIngestKey(key).
			SetNillablePreset(inb.preset).
			SetEnabled(enabled).
			SetIsDefault(isDefault).
			SetNillableMaxPerSecond(in.MaxPerSecond).
			SetNillableMaxPerDay(in.MaxPerDay).
			SetNillableProviderMaxPerSecond(providerPerSecond)
		if inb.sealed != nil {
			create.SetParamsEncrypted(*inb.sealed)
		}
		if err := constraints.Write(ctx, ts, func() (err error) {
			row, err = create.Save(ctx)
			return err
		}); err != nil {
			return err
		}
		return m.discoverQuota(ctx, q, row)
	}
	if in.IsDefault {
		// A new default moves every send of its Channel that names no Integration onto it, which
		// the Removal check of a deletion reads under the Workspace's lock, so it takes the lock (ADR
		// 0108) and clears the channel's current default first, in the same transaction, so the
		// partial unique index never trips on our own writes.
		err = m.bus.WithinWorkspaceLockEnqueueTx(ctx, s, func(wl *ent.WorkspaceLocked, _ events.Publisher, q events.Enqueuer) error {
			if err := clearDefault(ctx, wl, channel, 0); err != nil {
				return err
			}
			return insert(wl.Scope(), q, true)
		})
	} else {
		err = m.bus.WithinScopedEnqueueTx(ctx, s, func(ts *ent.Scoped, _ events.Publisher, q events.Enqueuer) error {
			return insert(ts, q, false)
		})
	}
	if err != nil {
		if provider == providers.Telegram && enabled {
			// The bot was pointed at a hook that was never saved.
			m.releaseToken(ctx, 0, tokenOf(in.Config.Telegram))
		}
		return View{}, err
	}
	return m.viewOf(ctx, s, row)
}

// Update applies a partial edit and answers the View of the result. An unknown id, or a deleted Integration's, is ErrNotFound.
func (m *Module) Update(ctx context.Context, s *ent.Scoped, id int64, in UpdateInput) (View, error) {
	row, err := m.editable(ctx, s, id)
	if err != nil {
		return View{}, err
	}

	// Validate and collect the changes up front so a refusal short-circuits before the
	// transaction opens.
	var name *string
	if raw, ok := in.Name.Get(); ok {
		n, err := validation.Name(raw)
		if err != nil {
			return View{}, err
		}
		name = &n
	}
	if err := checkProviderLimit(in.MaxPerSecond.Ptr(), in.MaxPerDay.Ptr()); err != nil {
		return View{}, err
	}
	var sealed *string
	var tg telegramEdit
	if row.Provider == providers.Telegram {
		// The bot is connected, as the edit asks, before the save commits.
		if tg, err = m.editTelegram(ctx, row, in); err != nil {
			return View{}, err
		}
		sealed = tg.sealed
	} else if row.Provider == providers.Twilio {
		// New credentials or a new sender are checked with Twilio before the save commits.
		if sealed, err = m.editTwilio(ctx, row, in); err != nil {
			return View{}, err
		}
	} else if sealed, err = m.sealEditedConfig(row, in.Config.Ptr()); err != nil {
		return View{}, err
	}
	if err := m.keepsProviderLimit(row, in); err != nil {
		return View{}, err
	}

	params, _ := in.Params.Get()
	inb, err := m.sealInbound(row, inboundChange{preset: in.Preset, params: params, retireSecrets: in.RetireSecrets})
	if err != nil {
		return View{}, err
	}

	// The save holds the Workspace's lock (ADR 0108): a promotion to default and a new config, which
	// may change the From its sends go out with, move Live use, which the Removal check of a deletion
	// reads under the lock. A deletion that holds it first leaves no row to edit.
	var updated *ent.Integration
	promote, _ := in.IsDefault.Get()
	err = m.bus.WithinWorkspaceLockEnqueueTx(ctx, s, func(wl *ent.WorkspaceLocked, _ events.Publisher, q events.Enqueuer) error {
		var err error
		updated, err = saveEdit(ctx, wl, row, promote, func(upd *ent.IntegrationScopedUpdateOne) {
			applyEdit(upd, in, name, sealed, inb)
			if tg.connected {
				upd.SetProviderMaxPerSecond(telegramPerSecond)
			}
		})
		if err != nil || sealed == nil {
			return err
		}
		// The provider's refusal of the old credentials goes with them.
		if err := renewCredentials(ctx, wl.Scope(), updated); err != nil {
			return err
		}
		if updated, err = wl.Integration().Get(ctx, row.ID); err != nil {
			return err
		}
		// New credentials or endpoint: what the provider allows may have changed.
		return m.discoverQuota(ctx, q, updated)
	})
	if err != nil {
		return View{}, missing(err)
	}
	if tg.release {
		m.release(ctx, updated)
	}
	return m.viewOf(ctx, s, updated)
}

// saveEdit writes an Update to the Integration row: apply sets what it changes and promote makes it
// its Channel's default. A promotion and a new config move Live use, so it takes the Workspace's
// lock (ADR 0108). A promotion clears the channel's current default first, in the same transaction.
// A Delete that landed meanwhile leaves no row to edit (ent's not found).
func saveEdit(ctx context.Context, wl *ent.WorkspaceLocked, row *ent.Integration, promote bool, apply func(*ent.IntegrationScopedUpdateOne)) (*ent.Integration, error) {
	if promote {
		if err := clearDefault(ctx, wl, row.Channel, row.ID); err != nil {
			return nil, err
		}
	}
	upd := wl.Integration().UpdateOneID(row.ID).Where(integration.DiscardedAtIsNil())
	apply(upd)
	var updated *ent.Integration
	err := constraints.Write(ctx, wl, func() (err error) {
		updated, err = upd.Save(ctx)
		return err
	})
	return updated, err
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
		return nil, refuse(fieldConfig, i18n.ErrorsIntegrationKindMismatch, nil)
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
		// provider's refusal of the old credentials is lifted after the save (renewCredentials).
		upd.SetConfigEncrypted(*sealed).ClearProviderEnforcement().SetProviderSandbox(false)
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

// ReleaseDeleted lets the bot of a Telegram Integration that held its webhook go of it, best
// effort, once the Integration's deletion has committed; row is the Integration as the deletion
// read it. Any other Integration holds nothing to release.
func (m *Module) ReleaseDeleted(ctx context.Context, row *ent.Integration) {
	if row.Provider == providers.Telegram && holdsWebhook(row.Enabled, nil) {
		m.release(ctx, row)
	}
}

// requireProviderLimit refuses an SES Integration with its own endpoint that is left without
// either Provider limit value (ADR 0067): an SES-compatible service has no quota call, so the
// customer gives what their provider account allows. AWS SES (no endpoint) reports its own.
func requireProviderLimit(endpoint string, perSecond, perDay *int) error {
	if endpoint == "" {
		return nil
	}
	var missing []error
	if perSecond == nil {
		missing = append(missing, ErrProviderLimitRequired.On(validation.FieldMaxPerSecond))
	}
	if perDay == nil {
		missing = append(missing, ErrProviderLimitRequired.On(validation.FieldMaxPerDay))
	}
	return validation.Fields(missing...)
}

// ErrProviderLimitRequired is a Provider limit an SES Integration with a custom endpoint leaves out.
var ErrProviderLimitRequired = validation.Refuse("integrations: the provider limit is required",
	i18n.ErrorsIntegrationProviderLimitRequired)

// checkProviderLimit refuses a Provider limit outside the contract's bounds; nil leaves it unset.
func checkProviderLimit(perSecond, perDay *int) error {
	var refusals []error
	if perSecond != nil {
		refusals = append(refusals, validation.Range(validation.FieldMaxPerSecond, *perSecond,
			bounds.MaxPerSecondMin, bounds.MaxPerSecondMax))
	}
	if perDay != nil {
		refusals = append(refusals, validation.Range(validation.FieldMaxPerDay, *perDay,
			bounds.MaxPerDayMin, bounds.MaxPerDayMax))
	}
	return validation.Fields(refusals...)
}

// refuse is the refusal of the request field named field, worded by key with args.
func refuse(field string, key i18n.ID, args map[string]any) error {
	return validation.RefuseField(field, "integrations: "+key.Key(), key).Args(args)
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

// discoverQuota queues a saved Integration's send quota lookup (ADR 0023) on q, the transaction of
// the save, when its provider can report one (SES; the Test inbox its own ceiling), so the lookup
// is queued iff the save commits. Providers without a quota are skipped.
func (m *Module) discoverQuota(ctx context.Context, q events.Enqueuer, row *ent.Integration) error {
	if !m.reportsQuota(row) {
		return nil
	}
	return q.Enqueue(ctx, RefreshQuotaArgs{WorkspaceID: row.WorkspaceID, IntegrationID: row.ID}, nil)
}

// reportsQuota reports whether the Integration's provider can say what it allows: whether its
// sender is a messaging.QuotaReader.
func (m *Module) reportsQuota(row *ent.Integration) bool {
	if m.catalog == nil {
		return false
	}
	_, ok, err := m.quotaReader(row)
	return err == nil && ok
}

// clearDefault unsets the channel's default Integration unless it is promoted, the one about to be
// made default (0 for a new one), under the Workspace's lock the promotion needs (ADR 0108).
func clearDefault(ctx context.Context, l *ent.WorkspaceLocked, channel channels.Channel, promoted int64) error {
	_, err := l.Integration().Update().
		Where(integration.ChannelEQ(channel), integration.IsDefault(true), integration.IDNEQ(promoted)).
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
		plaintext, err = json.Marshal(twilioStored(in.Twilio))
		return providers.Twilio, providers.Twilio.Channel(), plaintext, err
	case in.Telegram != nil:
		// A blank token is an update that keeps the stored one; the bot's id and @username come from
		// getMe when it connects.
		token := tokenOf(in.Telegram)
		if token != "" && !telegram.ValidToken(token) {
			return "", "", nil, tokenFormatError()
		}
		plaintext, err = json.Marshal(telegram.Config{Token: token})
		return providers.Telegram, providers.Telegram.Channel(), plaintext, err
	default:
		return "", "", nil, configError(errUnknownProvider)
	}
	if err != nil {
		return "", "", nil, err
	}

	if _, ok := m.catalog.Descriptor(provider); !ok {
		return "", "", nil, configError(errUnknownProvider)
	}
	if err := m.catalog.Validate(provider, plaintext); err != nil {
		return "", "", nil, configError(err)
	}
	return provider, provider.Channel(), plaintext, nil
}

// configError is a provider config the catalog refuses, worded by the catalog's own reason.
func configError(err error) error {
	return refuse(fieldConfig, i18n.ErrorsIntegrationConfigInvalid, map[string]any{detailKey: err.Error()})
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

func secretRequired() error {
	return refuse(fieldConfig, i18n.ErrorsIntegrationSecretRequired, nil)
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
