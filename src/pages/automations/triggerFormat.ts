// The schema `format` that marks a property as the Trigger's event.
export const TRIGGER_EVENT_FORMAT = 'trigger-event'
// The schema `format` of the Trigger's filter on the Event's properties (ADR 0133), JSON text.
export const TRIGGER_FILTER_FORMAT = 'trigger-filter'
// The schema `format`s of the Trigger's exit events and its goal event, Event matchers (#751, ADR 0133).
export const EXIT_EVENTS_FORMAT = 'exit-events'
export const GOAL_EVENT_FORMAT = 'goal-event'
// The schema `format`s of the Trigger's Entry policy and its cooldown in seconds (ADR 0127).
export const ENTRY_POLICY_FORMAT = 'entry-policy'
export const COOLDOWN_FORMAT = 'cooldown'
