package messaging

import (
	"context"
	"errors"
	"fmt"
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
	// SendMessage sends one text to a chat. A refusal of the Bot API is an error wrapping a
	// *TelegramError; a network error or a 5xx is a plain error, which a retry may get past.
	SendMessage(ctx context.Context, msg TelegramMessage) (TelegramSent, error)
}

// TelegramMessage is a sendMessage call: the chat it goes to (the Binding's chat id) and its text
// in Telegram's HTML subset, sent with parse_mode=HTML.
type TelegramMessage struct {
	ChatID string
	Text   string
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
