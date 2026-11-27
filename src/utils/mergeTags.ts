// The merge tags every send binds from the Contact's core fields (outboundmessages.Bindings, whose
// contactTag is the server's side of this vocabulary). A merge tag is no Condition attribute: the
// country tag renders the country's name, for one.
const contactTag = {
  firstName: 'first_name',
  lastName: 'last_name',
  email: 'email',
  city: 'city',
  country: 'country',
  locale: 'locale',
} as const

export const CONTACT_TAGS = Object.values(contactTag)

// The Telegram start link of the message's Contact, on the Workspace's default Telegram bot: it
// renders empty when the Workspace has none. A token is issued only for a message that uses it.
export const TELEGRAM_START_TAG = 'telegram_start_url'

// The action of the Event an Automation's Enrollment began on (#752). An Automation's messages bind
// that Event as `event`, the shape a Rule's HTTP action binds: `event.action` and `event.properties`.
export const EVENT = 'event'
export const EVENT_ACTION_TAG = `${EVENT}.action`

// The Event a Wait for event of the name arrived on, which an Enrollment keeps for its later Steps
// (ADR 0136): they bind it as `awaited.<name>`, the shape of `event`. It renders nothing where the
// Enrollment keeps no copy.
export function awaitedEvent(name: string): string {
  return `awaited.${name}`
}

// The merge tag of the action of that Event.
export function awaitedActionTag(name: string): string {
  return `${awaitedEvent(name)}.action`
}

const PLAIN_NAME = /^[A-Za-z_][A-Za-z0-9_]*$/

// A Liquid string literal has no escapes, and `}` would end the tag early.
const UNQUOTABLE = /["}]/

// The merge tag of one property of an Event a message binds (`event`, or an awaitedEvent): dotted
// for a plain name, bracketed for any other (a name with a space or a dash), null for a blank name
// or one a tag cannot quote.
export function propertyTag(source: string, name: string): string | null {
  const key = name.trim()
  if (key === '' || UNQUOTABLE.test(key)) {
    return null
  }
  return PLAIN_NAME.test(key) ? `${source}.properties.${key}` : `${source}.properties["${key}"]`
}

// The source-scoped unsubscribe link a Telegram message may carry (ADR 0078): it opts the chat out
// of the message's source on Telegram only. Email keeps its footer.
export const UNSUBSCRIBE_URL_TAG = 'unsubscribe_url'

// What an Automation's message binds beyond the Contact's, given the names of the Wait for events
// that can come before its Step: the merge tags of the Events' actions, and the Events whose
// properties it reads by name (`event`, then each `awaited.<name>`).
export function automationTags(awaited: readonly string[]): {
  tags: string[]
  sources: string[]
} {
  return {
    tags: [EVENT_ACTION_TAG, ...awaited.map((name) => awaitedActionTag(name))],
    sources: [EVENT, ...awaited.map((name) => awaitedEvent(name))],
  }
}
