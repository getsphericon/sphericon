// Package telegram implements the Telegram Channel's provider: a Workspace's own bot, reached
// through the Bot API with github.com/mymmrac/telego at the Instance's Bot API base URL
// (SPHERICON_TELEGRAM_BOT_API_URL). Its config is the bot token, plus the bot's id and @username
// as getMe last reported them.
package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"

	"github.com/getsphericon/sphericon/internal/messaging"
)

// DefaultAPIURL is Telegram's own Bot API server.
const DefaultAPIURL = "https://api.telegram.org"

// timeout bounds one Bot API call.
const timeout = 15 * time.Second

// Config is the stored config of a Telegram Integration. Token is the secret; BotID and Username
// are what getMe said when the Integration was last connected.
type Config struct {
	Token    string `json:"token"`
	BotID    int64  `json:"bot_id,omitempty"`
	Username string `json:"username,omitempty"`
}

// tokenFormat is a bot token as @BotFather issues it: the bot id, a colon and 35 characters.
var tokenFormat = regexp.MustCompile(`^\d+:[\w-]{35}$`)

// ErrTokenFormat means the token is not shaped like a bot token.
var ErrTokenFormat = errors.New("this is not a Telegram bot token: it looks like 123456789:AA… as @BotFather gives it")

// ValidToken reports whether token is shaped like a bot token.
func ValidToken(token string) bool { return tokenFormat.MatchString(token) }

// Descriptor registers the Telegram provider with the messaging catalog; apiURL is the Bot API
// base URL every bot of the Instance is reached at.
func Descriptor(apiURL string) messaging.ProviderDescriptor {
	return messaging.ProviderDescriptor{
		Channel:  messaging.ChannelTelegram,
		Provider: messaging.ProviderTelegram,
		Validate: func(config []byte) error {
			c, err := parse(config)
			if err != nil {
				return err
			}
			if !ValidToken(c.Token) {
				return ErrTokenFormat
			}
			return nil
		},
		Build: func(config []byte, _ messaging.Deps) (any, error) {
			c, err := parse(config)
			if err != nil {
				return nil, err
			}
			return New(apiURL, c.Token)
		},
	}
}

func parse(config []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(config, &c); err != nil {
		return Config{}, fmt.Errorf("telegram: read config: %w", err)
	}
	return c, nil
}

// Bot is one bot on the Bot API.
type Bot struct {
	bot *telego.Bot
}

// New builds the bot of token at the Bot API base URL apiURL.
func New(apiURL, token string) (*Bot, error) {
	if !ValidToken(token) {
		return nil, ErrTokenFormat
	}
	b, err := telego.NewBot(token,
		telego.WithAPIServer(strings.TrimRight(apiURL, "/")),
		telego.WithHTTPClient(&http.Client{Timeout: timeout}),
		telego.WithDiscardLogger(),
	)
	if err != nil {
		return nil, err
	}
	return &Bot{bot: b}, nil
}

// GetMe asks Telegram which bot the token belongs to.
func (b *Bot) GetMe(ctx context.Context) (messaging.TelegramBotInfo, error) {
	me, err := b.bot.GetMe(ctx)
	if err != nil {
		return messaging.TelegramBotInfo{}, refusal(err)
	}
	return messaging.TelegramBotInfo{ID: me.ID, Username: me.Username}, nil
}

// SetWebhook points the bot's updates at hook.URL.
func (b *Bot) SetWebhook(ctx context.Context, hook messaging.TelegramWebhook) error {
	return refusal(b.bot.SetWebhook(ctx, &telego.SetWebhookParams{
		URL:                hook.URL,
		SecretToken:        hook.SecretToken,
		AllowedUpdates:     hook.AllowedUpdates,
		DropPendingUpdates: hook.DropPendingUpdates,
	}))
}

// DeleteWebhook removes the bot's webhook, keeping its pending updates.
func (b *Bot) DeleteWebhook(ctx context.Context) error {
	return refusal(b.bot.DeleteWebhook(ctx, &telego.DeleteWebhookParams{}))
}

// refusal turns telego's error into a messaging.TelegramError when the Bot API answered one; any
// other error (the network, a 5xx) is returned as it is.
func refusal(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := errors.AsType[*ta.Error](err)
	if !ok {
		return err
	}
	out := &messaging.TelegramError{Code: apiErr.ErrorCode, Description: apiErr.Description}
	if apiErr.Parameters != nil {
		out.RetryAfter = time.Duration(apiErr.Parameters.RetryAfter) * time.Second
	}
	return out
}
