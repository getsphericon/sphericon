// The System events of what happens to a Contact (#897), which pickers offer wherever an action
// can start or end something: a Trigger, an exit or goal event, a webhook's subscription.

// What happened to a Contact's Tags and Custom fields.
export const CONTACT_CHANGE_ACTIONS = [
  'contact.tagged',
  'contact.untagged',
  'contact.field_changed',
] as const

// An Enrollment's beginning and its ends (a failed one records none).
export const ENROLLMENT_ACTIONS = [
  'automation.enrolled',
  'automation.completed',
  'automation.goal_reached',
  'automation.exited',
] as const

// A Telegram chat bound to a Contact, one moved to another Contact, and a message a bound Contact
// sent the bot (ADR 0076), a Contact stopping Telegram marketing there (ADR 0077) and one clicking a
// link of a message (ADR 0083).
export const TELEGRAM_ACTIONS = [
  'telegram.bound',
  'telegram.clicked',
  'telegram.received',
  'telegram.unbound',
  'telegram.unsubscribed',
] as const

// A Contact clicking a link of a text (ADR 0083), giving and withdrawing its consent to SMS
// marketing (ADR 0077), a text it sent from its phone (ADR 0076), and whether a text reached its
// phone, as the provider reported it (ADR 0079).
export const SMS_ACTIONS = [
  'sms.clicked',
  'sms.confirmed',
  'sms.delivered',
  'sms.received',
  'sms.undelivered',
  'sms.unsubscribed',
] as const
