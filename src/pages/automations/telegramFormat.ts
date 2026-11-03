// The schema `format` that marks a property as the text of a Telegram step.
export const TELEGRAM_TEXT_FORMAT = 'telegram-text'

// The most characters Telegram takes in a message; the counter and Publish use it.
export const TELEGRAM_TEXT_LIMIT = 4096

// The schema `format` that marks a property as the label of a Telegram step's unsubscribe button.
export const TELEGRAM_UNSUBSCRIBE_FORMAT = 'telegram-unsubscribe-button'

// The longest label the button takes; the API refuses a longer one.
export const TELEGRAM_UNSUBSCRIBE_LIMIT = 64
