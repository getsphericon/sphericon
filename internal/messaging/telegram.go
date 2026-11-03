package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TelegramBot is the Telegram Channel's provider: one Workspace's bot, reached through the Bot
// API at the Instance's Bot API base URL. Connecting an Integration asks the bot who it is and
// points its webhook at the Integration's Ingest hook.
type TelegramBot interface {
	// GetMe is the bot the token belongs to. A token Telegram does not accept is an error wrapping
	// ErrTelegramUnauthorized.
	GetMe(ctx context.Context) (TelegramBotInfo, error)
	// SetWebhook points the bot's updates at a URL, replacing any webhook it had.
	SetWebhook(ctx context.Context, hook TelegramWebhook) error
	// DeleteWebhook stops the bot's updates from reaching the webhook. Pending updates are kept.
	DeleteWebhook(ctx context.Context) error
	// SendMessage sends one message to a chat. A refusal of the Bot API is an error wrapping a
	// *TelegramError; a network error or a 5xx is a plain error, which a retry may get past.
	SendMessage(ctx context.Context, msg TelegramMessage) (TelegramSent, error)
	// AnswerCallbackQuery tells Telegram a button press was received, so the person's client stops
	// waiting. It shows the person nothing.
	AnswerCallbackQuery(ctx context.Context, callbackID string) error
}

// TelegramUnsubscribePayload is the fixed payload of the unsubscribe button's callback (ADR 0076).
const TelegramUnsubscribePayload = "unsubscribe"

// TelegramMessage is a message to one chat (the Binding's chat id), its text in Telegram's HTML
// subset sent with parse_mode=HTML. With an ImageURL it goes by sendPhoto and the text is the
// caption; otherwise by sendMessage. Buttons are inline URL buttons, one per row.
type TelegramMessage struct {
	ChatID   string
	Text     string
	ImageURL string
	Buttons  []TelegramButton
	// NoLinkPreview turns the link preview off (link_preview_options.is_disabled). A photo has none.
	NoLinkPreview bool
	// UnsubscribeButton is the label of a callback button, with the payload
	// TelegramUnsubscribePayload, added as the last row of the message's buttons; "" adds none.
	UnsubscribeButton string
}

// TelegramButton is an inline URL button.
type TelegramButton struct {
	Label string
	URL   string
}

// TelegramSent is what Telegram answers to an accepted message: its id in the chat.
type TelegramSent struct {
	MessageID int64
}

// TelegramBotInfo is what getMe says of a bot: its numeric id and its @username (without the @).
type TelegramBotInfo struct {
	ID       int64
	Username string
}

// TelegramWebhook is a setWebhook call: where updates go, the secret Telegram sends back in the
// X-Telegram-Bot-Api-Secret-Token header, the update kinds delivered and whether the updates
// waiting for the bot are dropped.
type TelegramWebhook struct {
	URL                string
	SecretToken        string
	AllowedUpdates     []string
	DropPendingUpdates bool
}

// ErrTelegramUnauthorized is Telegram's 401: the token is wrong or revoked.
var ErrTelegramUnauthorized = errors.New("telegram: the bot token is not accepted")

// TelegramError is a refusal of the Bot API: its error code, its description in Telegram's own
// words and, on a 429, how long to wait before the next call.
type TelegramError struct {
	Code        int
	Description string
	RetryAfter  time.Duration
}

func (e *TelegramError) Error() string {
	return fmt.Sprintf("telegram: %d %s", e.Code, e.Description)
}

// Is makes a 401 match ErrTelegramUnauthorized.
func (e *TelegramError) Is(target error) bool {
	return target == ErrTelegramUnauthorized && e.Code == 401
}

// Blocked reports whether the Bot API refused a send because the person blocked the bot: a 403
// "Forbidden: bot was blocked by the user".
func (e *TelegramError) Blocked() bool {
	return e.Code == 403 && strings.Contains(strings.ToLower(e.Description), "bot was blocked by the user")
}
