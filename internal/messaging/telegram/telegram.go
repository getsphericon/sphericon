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
	"strconv"
	"strings"
	"time"

	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"

	"github.com/getsphericon/sphericon/internal/kernel/channels"
	"github.com/getsphericon/sphericon/internal/kernel/providers"
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
		Channel:  channels.Telegram,
		Provider: providers.Telegram,
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
	bot   *telego.Bot
	token string
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
	return &Bot{bot: b, token: token}, nil
}

// GetMe asks Telegram which bot the token belongs to.
func (b *Bot) GetMe(ctx context.Context) (messaging.TelegramBotInfo, error) {
	me, err := b.bot.GetMe(ctx)
	if err != nil {
		return messaging.TelegramBotInfo{}, b.refusal(err)
	}
	return messaging.TelegramBotInfo{ID: me.ID, Username: me.Username}, nil
}

// SetWebhook points the bot's updates at hook.URL.
func (b *Bot) SetWebhook(ctx context.Context, hook messaging.TelegramWebhook) error {
	return b.refusal(b.bot.SetWebhook(ctx, &telego.SetWebhookParams{
		URL:                hook.URL,
		SecretToken:        hook.SecretToken,
		AllowedUpdates:     hook.AllowedUpdates,
		DropPendingUpdates: hook.DropPendingUpdates,
	}))
}

// DeleteWebhook removes the bot's webhook, keeping its pending updates.
func (b *Bot) DeleteWebhook(ctx context.Context) error {
	return b.refusal(b.bot.DeleteWebhook(ctx, &telego.DeleteWebhookParams{}))
}

// SendMessage sends msg to the chat msg.ChatID, as HTML: by sendPhoto with the text as the caption
// when it has an image, else by sendMessage.
func (b *Bot) SendMessage(ctx context.Context, msg messaging.TelegramMessage) (messaging.TelegramSent, error) {
	chat, err := strconv.ParseInt(msg.ChatID, 10, 64)
	if err != nil {
		return messaging.TelegramSent{}, fmt.Errorf("telegram: chat id %q is not a number", msg.ChatID)
	}
	var keyboard *telego.InlineKeyboardMarkup
	if len(msg.Buttons) > 0 || msg.UnsubscribeButton != "" {
		rows := make([][]telego.InlineKeyboardButton, 0, len(msg.Buttons)+1)
		for _, btn := range msg.Buttons {
			rows = append(rows, []telego.InlineKeyboardButton{{Text: btn.Label, URL: btn.URL}})
		}
		if msg.UnsubscribeButton != "" {
			rows = append(rows, []telego.InlineKeyboardButton{{Text: msg.UnsubscribeButton, CallbackData: messaging.TelegramUnsubscribePayload}})
		}
		keyboard = &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
	}
	var sent *telego.Message
	if msg.ImageURL != "" {
		sent, err = b.bot.SendPhoto(ctx, &telego.SendPhotoParams{
			ChatID:      telego.ChatID{ID: chat},
			Photo:       telego.InputFile{URL: msg.ImageURL},
			Caption:     msg.Text,
			ParseMode:   telego.ModeHTML,
			ReplyMarkup: keyboard,
		})
	} else {
		params := &telego.SendMessageParams{
			ChatID:      telego.ChatID{ID: chat},
			Text:        msg.Text,
			ParseMode:   telego.ModeHTML,
			ReplyMarkup: keyboard,
		}
		if msg.NoLinkPreview {
			params.LinkPreviewOptions = &telego.LinkPreviewOptions{IsDisabled: true}
		}
		sent, err = b.bot.SendMessage(ctx, params)
	}
	if err != nil {
		return messaging.TelegramSent{}, b.refusal(err)
	}
	return messaging.TelegramSent{MessageID: int64(sent.MessageID)}, nil
}

// AnswerCallbackQuery answers a button press with nothing to show.
func (b *Bot) AnswerCallbackQuery(ctx context.Context, callbackID string) error {
	return b.refusal(b.bot.AnswerCallbackQuery(ctx, &telego.AnswerCallbackQueryParams{CallbackQueryID: callbackID}))
}

// refusal turns telego's error into a messaging.TelegramError when the Bot API answered one. Any
// other error (the network, a 5xx) is returned with the bot token taken out of its text: telego
// wraps the transport's *url.Error, whose URL is .../bot<token>/method, and the text reaches the
// user, the logs and the retried send's error. The wrapped chain stays, so errors.Is/As still work.
func (b *Bot) refusal(err error) error {
	if err == nil {
		return nil
	}
	apiErr, ok := errors.AsType[*ta.Error](err)
	if !ok {
		return redacted{msg: strings.ReplaceAll(err.Error(), b.token, "<token>"), cause: err}
	}
	out := &messaging.TelegramError{Code: apiErr.ErrorCode, Description: apiErr.Description}
	if apiErr.Parameters != nil {
		out.RetryAfter = time.Duration(apiErr.Parameters.RetryAfter) * time.Second
	}
	return out
}

// redacted is an error whose text has the bot token removed and whose cause is kept for Is and As.
type redacted struct {
	msg   string
	cause error
}

func (e redacted) Error() string { return e.msg }
func (e redacted) Unwrap() error { return e.cause }
