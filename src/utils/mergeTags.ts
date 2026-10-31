// The merge tags every send binds (outbound.Bindings): the Contact's core fields.
export const CONTACT_TAGS = ['first_name', 'last_name', 'email'] as const

// The action of the Event an Automation's Enrollment began on (#752). An Automation's messages bind
// that Event as `event`, the shape a Rule's HTTP action binds: `event.action` and `event.properties`.
export const EVENT_ACTION_TAG = 'event.action'

const PLAIN_NAME = /^[A-Za-z_][A-Za-z0-9_]*$/

// A Liquid string literal has no escapes, and `}` would end the tag early.
const UNQUOTABLE = /["}]/

// The merge tag of one property of that Event: dotted for a plain name, bracketed for any other
// (a name with a space or a dash), null for a blank name or one a tag cannot quote.
export function eventPropertyTag(name: string): string | null {
  const key = name.trim()
  if (key === '' || UNQUOTABLE.test(key)) {
    return null
  }
  return PLAIN_NAME.test(key) ? `event.properties.${key}` : `event.properties["${key}"]`
}
