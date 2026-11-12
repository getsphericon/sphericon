import { SystemEventAction } from '../generated/site/types.gen.ts'

// The System events of what happens to a Contact (#897), which pickers offer wherever an action
// can start or end something: a Trigger, an exit or goal event, a webhook's subscription.

// What happened to a Contact's Tags and Custom fields.
export const CONTACT_CHANGE_ACTIONS = [
  SystemEventAction.CONTACT_TAGGED,
  SystemEventAction.CONTACT_UNTAGGED,
  SystemEventAction.CONTACT_FIELD_CHANGED,
] as const

// An Enrollment's beginning and its ends (a failed one records none).
export const ENROLLMENT_ACTIONS = [
  SystemEventAction.AUTOMATION_ENROLLED,
  SystemEventAction.AUTOMATION_COMPLETED,
  SystemEventAction.AUTOMATION_GOAL_REACHED,
  SystemEventAction.AUTOMATION_EXITED,
] as const

// A Telegram chat bound to a Contact, one moved to another Contact, and a message a bound Contact
// sent the bot (ADR 0076), a Contact stopping Telegram marketing there (ADR 0077) and one clicking a
// link of a message (ADR 0083).
export const TELEGRAM_ACTIONS = [
  SystemEventAction.TELEGRAM_BOUND,
  SystemEventAction.TELEGRAM_CLICKED,
  SystemEventAction.TELEGRAM_RECEIVED,
  SystemEventAction.TELEGRAM_UNBOUND,
  SystemEventAction.TELEGRAM_UNSUBSCRIBED,
] as const

// A Contact clicking a link of a text (ADR 0083), giving and withdrawing its consent to SMS
// marketing (ADR 0077), a text it sent from its phone (ADR 0076), and whether a text reached its
// phone, as the provider reported it (ADR 0079).
export const SMS_ACTIONS = [
  SystemEventAction.SMS_CLICKED,
  SystemEventAction.SMS_CONFIRMED,
  SystemEventAction.SMS_DELIVERED,
  SystemEventAction.SMS_RECEIVED,
  SystemEventAction.SMS_UNDELIVERED,
  SystemEventAction.SMS_UNSUBSCRIBED,
] as const

// The email engagement a person does, as opposed to the delivery notices: what an Automation may
// start from and a webhook offers first (ADR 0053).
export const EMAIL_ENGAGEMENT_ACTIONS = [
  SystemEventAction.EMAIL_OPENED,
  SystemEventAction.EMAIL_CLICKED,
  SystemEventAction.EMAIL_UNSUBSCRIBED,
] as const

// The first-party tracker's page view: a customer's own action the tracker names, not a System
// event, which the app still words.
export const PAGE_VIEW_ACTION = 'page.view'

// The bus's Audit entry a webhook subscribes to (Enterprise): an event of the Workspace, not of a
// Contact, so no System event.
export const AUDIT_ENTRY_EVENT = 'audit.entry'

// An action the app words: a System event's, or one of the two above.
export type WordedAction = SystemEventAction | typeof PAGE_VIEW_ACTION | typeof AUDIT_ENTRY_EVENT
