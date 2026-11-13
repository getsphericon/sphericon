package integrations

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/integration"
	"github.com/getsphericon/sphericon/internal/domain/sending/preset"
	"github.com/getsphericon/sphericon/internal/infra/messaging"
	"github.com/getsphericon/sphericon/internal/infra/messaging/telegram"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
)

// A Telegram Integration is a Workspace's own bot (spec #962, "Channel and Integration"). Saving it
// connects the bot before the save commits: getMe says which bot the token is of, and setWebhook
// points the bot's updates at the Integration's Ingest hook. The Integration holds the bot's
// webhook while it is enabled and kept; every write that changes that, or the hook URL, calls
// Telegram.

// telegramPerSecond is Telegram's bulk limit of a bot: about 30 messages a second, stored as the
// provider's side of the Provider limit (ADR 0023) when the bot connects.
const telegramPerSecond = 30

// telegramUpdates are the update kinds the webhook asks for: messages (a /start, a reply), button
// presses and the bot's membership in a chat (a block).
var telegramUpdates = []string{"message", "callback_query", "my_chat_member"}

// telegramTimeout bounds the Bot API calls of one write.
const telegramTimeout = 30 * time.Second

// TelegramConfig is the write side of a Telegram Integration: the bot token. Blank on an update
// keeps the stored token.
type TelegramConfig struct {
	Token string
}

// RedactedTelegram is the read side: which bot it is, as getMe said when it was connected.
type RedactedTelegram struct {
	BotID    int64
	Username string
}

// telegramParams are the params of the telegram Preset: the webhook's secret token, which sphericon
// generates and never shows. rand.Text is base32, within what Telegram takes ([A-Za-z0-9_-]).
func telegramParams() map[string]any {
	return map[string]any{"secret": rand.Text() + rand.Text()}
}

// holdsWebhook reports whether an Integration in this state holds its bot's webhook: enabled and
// kept.
func holdsWebhook(enabled bool, archivedAt *time.Time) bool { return enabled && archivedAt == nil }

// telegramConfigOf opens a Telegram Integration's stored config.
func (m *Module) telegramConfigOf(row *ent.Integration) (telegram.Config, error) {
	plaintext, err := m.cipher.Decrypt(row.ConfigEncrypted)
	if err != nil {
		return telegram.Config{}, err
	}
	var c telegram.Config
	err = json.Unmarshal(plaintext, &c)
	return c, err
}

// bot builds the bot of token through the catalog.
func (m *Module) bot(token string) (messaging.TelegramBot, error) {
	raw, err := json.Marshal(telegram.Config{Token: token})
	if err != nil {
		return nil, err
	}
	return m.catalog.BuildTelegram(providers.Telegram, raw)
}

// Bot is the bot a Telegram Integration is, built from its stored token: what the Ingest hook
// answers a button press through.
func (m *Module) Bot(row *ent.Integration) (messaging.TelegramBot, error) {
	cfg, err := m.telegramConfigOf(row)
	if err != nil {
		return nil, fmt.Errorf("open the config of integration %d: %w", row.ID, err)
	}
	return m.bot(cfg.Token)
}

// identify asks Telegram which bot token is of. A token Telegram does not accept is a refusal on
// the token field; another refusal, or Telegram out of reach, is a refusal of the config.
func (m *Module) identify(ctx context.Context, token string) (messaging.TelegramBotInfo, error) {
	b, err := m.bot(token)
	if err != nil {
		return messaging.TelegramBotInfo{}, tokenFormatError()
	}
	ctx, cancel := context.WithTimeout(ctx, telegramTimeout)
	defer cancel()
	info, err := b.GetMe(ctx)
	if err != nil {
		return messaging.TelegramBotInfo{}, telegramRefusal(err, i18n.ErrorsIntegrationTelegramRefused)
	}
	return info, nil
}

// register points token's bot at the hook of key, with the Integration's secret. drop clears the
// updates waiting for the bot: only the first connect of a new Integration passes it, so that no
// queued /stop, block or button press of a known bot is lost later.
func (m *Module) register(ctx context.Context, token, key, secret string, drop bool) error {
	b, err := m.bot(token)
	if err != nil {
		return tokenFormatError()
	}
	ctx, cancel := context.WithTimeout(ctx, telegramTimeout)
	defer cancel()
	err = b.SetWebhook(ctx, messaging.TelegramWebhook{
		URL: m.hookURL(key), SecretToken: secret, AllowedUpdates: telegramUpdates, DropPendingUpdates: drop,
	})
	if err != nil {
		return telegramRefusal(err, i18n.ErrorsIntegrationTelegramWebhookRefused)
	}
	return nil
}

// release removes the webhook of a Telegram Integration's bot, best effort: a failure is logged
// and never blocks the write that let the bot go.
func (m *Module) release(ctx context.Context, row *ent.Integration) {
	cfg, err := m.telegramConfigOf(row)
	if err != nil {
		slog.WarnContext(ctx, "telegram: open the config to delete the bot's webhook failed", "integration_id", row.ID, "err", err)
		return
	}
	m.releaseToken(ctx, row.ID, cfg.Token)
}

// heldByAnother reports whether a kept, enabled Telegram Integration other than the one of the id,
// in any Workspace, is of the bot of token: one that holds the bot's webhook now. A bot is known by
// its id, the token's part before the colon, which a new token of the bot keeps. An Integration
// whose config does not open is passed over: it cannot be told apart.
func (m *Module) heldByAnother(ctx context.Context, client *ent.Client, id int64, token string) (bool, error) {
	kept, err := client.Integration.Query().
		Where(integration.ProviderEQ(providers.Telegram), integration.DiscardedAtIsNil(), integration.Enabled(true), integration.IDNEQ(id)).
		All(ctx)
	if err != nil {
		return false, err
	}
	bot := botOf(token)
	for _, other := range kept {
		cfg, err := m.telegramConfigOf(other)
		if err != nil {
			slog.WarnContext(ctx, "telegram: open the config of a kept bot failed", "integration_id", other.ID, "err", err)
			continue
		}
		if botOf(cfg.Token) == bot {
			return true, nil
		}
	}
	return false, nil
}

// botOf is the bot id a token is of: its part before the colon (telegram.ValidToken).
func botOf(token string) string {
	bot, _, _ := strings.Cut(strings.TrimSpace(token), ":")
	return bot
}

// releaseToken removes the webhook of token's bot, best effort.
func (m *Module) releaseToken(ctx context.Context, integrationID int64, token string) {
	b, err := m.bot(token)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), telegramTimeout)
		defer cancel()
		err = b.DeleteWebhook(ctx)
	}
	if err != nil {
		slog.WarnContext(ctx, "telegram: delete the bot's webhook failed", "integration_id", integrationID, "err", err)
	}
}

// connectNew connects the bot of a new Telegram Integration: getMe, then, when it is created
// enabled, setWebhook at the hook of key with drop_pending_updates (the first connect clears what
// another tool left queued). It returns the config to store, with the bot's id and @username.
func (m *Module) connectNew(ctx context.Context, token, key, secret string, enabled bool) ([]byte, error) {
	if token == "" || !telegram.ValidToken(token) {
		return nil, tokenFormatError()
	}
	info, err := m.identify(ctx, token)
	if err != nil {
		return nil, err
	}
	if enabled {
		if err := m.register(ctx, token, key, secret, true); err != nil {
			return nil, err
		}
	}
	return json.Marshal(telegram.Config{Token: token, BotID: info.ID, Username: info.Username})
}

// telegramEdit is what an Update of a Telegram Integration leaves to do: the config to store (nil:
// unchanged), whether the bot was connected (the Provider limit is set again) and whether the
// Integration let the bot's webhook go (removed after the commit).
type telegramEdit struct {
	sealed    *string
	connected bool
	release   bool
}

// editTelegram validates an Update of a Telegram Integration and connects its bot as the edit asks,
// before the save commits: a new token must be of the same bot (getMe), and an Integration that
// holds the webhook after the edit registers it again when the token changed or it was just
// enabled, keeping the pending updates. The same token saved again changes nothing, unless Telegram
// refused it at a send: the Owner saves it once the bot works again, and it is checked anew and the
// hold goes.
func (m *Module) editTelegram(ctx context.Context, row *ent.Integration, in UpdateInput) (telegramEdit, error) {
	params, _ := in.Params.Get()
	if name := in.Preset.Over(row.Preset); (in.Preset.Changes() && (name == nil || *name != preset.BuiltinTelegram)) || len(params) > 0 {
		return telegramEdit{}, refuse(fieldPreset, i18n.ErrorsIntegrationTelegramPresetFixed, nil)
	}
	stored, err := m.telegramConfigOf(row)
	if err != nil {
		return telegramEdit{}, err
	}
	next := stored
	cfg, configGiven := in.Config.Get()
	if configGiven {
		if cfg.Telegram == nil {
			return telegramEdit{}, refuse(fieldConfig, i18n.ErrorsIntegrationKindMismatch, nil)
		}
		if token := tokenOf(cfg.Telegram); token != "" {
			if !telegram.ValidToken(token) {
				return telegramEdit{}, tokenFormatError()
			}
			next.Token = token
		}
	}
	tokenChanged := next.Token != stored.Token
	was := holdsWebhook(row.Enabled, row.DiscardedAt)
	will := holdsWebhook(*in.Enabled.Over(&row.Enabled), row.DiscardedAt)
	connect := will && (tokenChanged || !was)
	recheck := configGiven && row.ProviderRefusedAt != nil

	if tokenChanged || connect || recheck {
		info, err := m.identify(ctx, next.Token)
		if err != nil {
			return telegramEdit{}, err
		}
		if info.ID != stored.BotID {
			return telegramEdit{}, refuse(fieldToken, i18n.ErrorsIntegrationTelegramOtherBot, nil)
		}
		next.Username = info.Username
	}
	if connect {
		if err := m.reconnect(ctx, row, next.Token, row.IngestKey); err != nil {
			return telegramEdit{}, err
		}
	}
	out := telegramEdit{connected: connect, release: was && !will}
	// A config sealed anew is what lifts the hold (applyEdit), so a re-checked one is stored again.
	if next != stored || recheck {
		raw, err := json.Marshal(next)
		if err != nil {
			return telegramEdit{}, err
		}
		enc, err := m.cipher.Encrypt(raw)
		if err != nil {
			return telegramEdit{}, err
		}
		out.sealed = &enc
	}
	return out, nil
}

// secretOf is the webhook secret a Telegram Integration's params hold.
func (m *Module) secretOf(row *ent.Integration) (string, error) {
	params, err := m.Params(row)
	if err != nil {
		return "", err
	}
	return currentSecret(params["secret"]), nil
}

// reconnect registers a stored Telegram Integration's bot at the hook of key: the webhook of a
// bot that was connected before, so the pending updates stay.
func (m *Module) reconnect(ctx context.Context, row *ent.Integration, token, key string) error {
	secret, err := m.secretOf(row)
	if err != nil {
		return err
	}
	return m.register(ctx, token, key, secret, false)
}

// telegramRefusal turns a Bot API failure into a refusal of the write: a 401 on the token field,
// any other answer of Telegram in its own words (under id), and an unreachable Telegram as such.
func telegramRefusal(err error, id i18n.ID) error {
	if errors.Is(err, messaging.ErrTelegramUnauthorized) {
		return refuse(fieldToken, i18n.ErrorsIntegrationTelegramTokenRefused, nil)
	}
	if tg, ok := errors.AsType[*messaging.TelegramError](err); ok {
		return refuse(fieldConfig, id, map[string]any{detailKey: tg.Description})
	}
	return refuse(fieldConfig, i18n.ErrorsIntegrationTelegramUnreachable, map[string]any{detailKey: err.Error()})
}

// fieldToken is the field a refusal of the bot token is on.
const fieldToken = "config.token"

func tokenFormatError() error {
	return refuse(fieldToken, i18n.ErrorsIntegrationTelegramTokenFormat, nil)
}

// presetFits refuses a Preset or params a write names that do not fit the provider: a Telegram
// Integration takes neither (its Preset is fixed), a Twilio Integration only its own Preset's params,
// and no other Integration takes the telegram or the twilio Preset.
func presetFits(provider providers.Provider, name *string, params map[string]any) error {
	switch provider {
	case providers.Telegram:
		if name != nil || len(params) > 0 {
			return refuse(fieldPreset, i18n.ErrorsIntegrationTelegramPresetFixed, nil)
		}
		return nil
	case providers.Twilio:
		if name != nil && *name != preset.BuiltinTwilio {
			return refuse(fieldPreset, i18n.ErrorsIntegrationTwilioPresetFixed, nil)
		}
		return nil
	case providers.SMTP, providers.SES, providers.Inbox:
	}
	if name != nil && *name == preset.BuiltinTelegram {
		return refuse(fieldPreset, i18n.ErrorsIntegrationPresetTelegramOnly, nil)
	}
	if name != nil && *name == preset.BuiltinTwilio {
		return refuse(fieldPreset, i18n.ErrorsIntegrationPresetTwilioOnly, nil)
	}
	return nil
}

// Choosable is the Presets a caller may name on an Integration, by name: every Preset of the
// Instance but the telegram one, which is fixed to Telegram Integrations and has no param to show.
// The twilio Preset is listed for its params (a Twilio Integration's Auth Token), though only a
// Twilio Integration takes it.
func (m *Module) Choosable() []*preset.Preset {
	names := m.presets.Names()
	out := make([]*preset.Preset, 0, len(names))
	for _, name := range names {
		if name == preset.BuiltinTelegram {
			continue
		}
		p, _ := m.presets.Get(name)
		out = append(out, p)
	}
	return out
}

// tokenOf is the token a Telegram config input names, trimmed.
func tokenOf(c *TelegramConfig) string { return strings.TrimSpace(c.Token) }
