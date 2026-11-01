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
