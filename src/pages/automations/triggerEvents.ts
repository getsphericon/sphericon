import { SystemEventAction } from '../../generated/site/types.gen.ts'
import {
  CONTACT_CHANGE_ACTIONS,
  EMAIL_ENGAGEMENT_ACTIONS,
  ENGAGEMENT_TRACKING_ACTIONS,
  ENROLLMENT_ACTIONS,
  SMS_ACTIONS,
  TELEGRAM_ACTIONS,
} from '../../utils/systemEvents.ts'

// The built-in events a Trigger, an exit or a goal suggests; any other action is typed in.
export const TRIGGER_EVENTS = [
  SystemEventAction.CONTACT_CREATED,
  ...CONTACT_CHANGE_ACTIONS,
  ...EMAIL_ENGAGEMENT_ACTIONS,
  ...ENROLLMENT_ACTIONS,
  ...TELEGRAM_ACTIONS,
  ...SMS_ACTIONS,
  ...ENGAGEMENT_TRACKING_ACTIONS,
] as const
export const DEFAULT_TRIGGER_EVENT = TRIGGER_EVENTS[0]
