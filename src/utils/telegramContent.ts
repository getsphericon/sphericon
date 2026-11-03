import type { TelegramButton } from '../generated/site/types.gen.ts'

// The most characters Telegram takes in a text message, and in the caption of a photo.
const TELEGRAM_TEXT_LIMIT = 4096
const TELEGRAM_CAPTION_LIMIT = 1024
// The most link buttons a message carries.
export const TELEGRAM_MAX_BUTTONS = 8

// Telegram's message content (ADR 0078) as the editor holds it. The Automation step and the
// Broadcast both edit this value.
export interface TelegramContentValue {
  text: string
  // An https image URL, '' for a text message.
  imageUrl: string
  buttons: TelegramButton[]
  linkPreview: boolean
}

// The limit that applies: a caption with an image, a text without one.
export function telegramLimit(imageUrl: string): number {
  return imageUrl.trim() === '' ? TELEGRAM_TEXT_LIMIT : TELEGRAM_CAPTION_LIMIT
}

// The sample Contact the preview resolves merge tags for.
const SAMPLE_CONTACT: Record<string, string> = {
  first_name: 'Alex',
  last_name: 'Morgan',
  email: 'alex@example.com',
  city: 'Berlin',
  country: 'Germany',
  locale: 'en',
  'event.action': 'order_placed',
  unsubscribe_url: 'https://example.com/e/u/…',
  telegram_start_url: 'https://t.me/your_bot?start=…',
}

const TAG = /\{\{\s*([\w.]+)\s*\}\}/g

const ESCAPES: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;' }

// resolveTags writes the sample values where the text names a merge tag, escaped as the server
// escapes a value; a tag the sample does not know renders empty, as an unbound one does.
export function resolveTags(text: string): string {
  return text.replace(TAG, (_, name: string) =>
    (SAMPLE_CONTACT[name] ?? '').replace(/[&<>]/g, (c) => ESCAPES[c] ?? c),
  )
}

// The buttons a draft may send: those with a label and an URL (a half-typed row is dropped).
export function completeButtons(buttons: TelegramButton[]): TelegramButton[] {
  return buttons.filter((b) => b.label.trim() !== '' && b.url.trim() !== '')
}
