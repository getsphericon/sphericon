import type { TelegramButton } from '../generated/site/types.gen.ts'
import { zTelegramBroadcastContentInput } from '../generated/site/zod.gen.ts'
import { maxItemsOf, maxLengthOf } from './bounds.ts'
import { EVENT_ACTION_TAG, TELEGRAM_START_TAG, UNSUBSCRIBE_URL_TAG } from './mergeTags.ts'
import type { CONTACT_TAGS } from './mergeTags.ts'

// The most characters Telegram takes in a text message, and in the caption of a photo.
// Telegram's own limits, which the editor counts live; the API refuses a longer text
const TELEGRAM_TEXT_LIMIT = 4096
// Telegram's own limit, counted live in the editor; the API refuses a longer caption
const TELEGRAM_CAPTION_LIMIT = 1024
// The longest label an unsubscribe button takes, and the most link buttons a message carries: the
// contract's bounds, so the editor stops before the API refuses.
export const TELEGRAM_UNSUBSCRIBE_LIMIT = maxLengthOf(
  zTelegramBroadcastContentInput.shape.unsubscribeButton.unwrap(),
)
export const TELEGRAM_MAX_BUTTONS = maxItemsOf(
  zTelegramBroadcastContentInput.shape.buttons.unwrap(),
)

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

// A merge tag the preview fills in: the Contact's core fields and the tags every Telegram message may
// carry.
type SampleTag =
  | (typeof CONTACT_TAGS)[number]
  | typeof EVENT_ACTION_TAG
  | typeof UNSUBSCRIBE_URL_TAG
  | typeof TELEGRAM_START_TAG

// The sample Contact the preview resolves merge tags for, one value per tag it knows.
const SAMPLE_CONTACT: Record<SampleTag, string> = {
  first_name: 'Alex',
  last_name: 'Morgan',
  email: 'alex@example.com',
  city: 'Berlin',
  country: 'Germany',
  locale: 'en',
  [EVENT_ACTION_TAG]: 'order_placed',
  [UNSUBSCRIBE_URL_TAG]: 'https://example.com/e/u/…',
  [TELEGRAM_START_TAG]: 'https://t.me/your_bot?start=…',
}

const isSampleTag = (name: string): name is SampleTag => Object.hasOwn(SAMPLE_CONTACT, name)

const TAG = /\{\{\s*([\w.]+)\s*\}\}/g

const ESCAPES: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;' }

// resolveTags writes the sample values where the text names a merge tag, escaped as the server
// escapes a value; a tag the sample does not know renders empty, as an unbound one does.
export function resolveTags(text: string): string {
  return text.replace(TAG, (_, name: string) =>
    (isSampleTag(name) ? SAMPLE_CONTACT[name] : '').replace(/[&<>]/g, (c) => ESCAPES[c] ?? c),
  )
}

// The buttons a draft may send: those with a label and an URL (a half-typed row is dropped).
export function completeButtons(buttons: TelegramButton[]): TelegramButton[] {
  return buttons.filter((b) => b.label.trim() !== '' && b.url.trim() !== '')
}
