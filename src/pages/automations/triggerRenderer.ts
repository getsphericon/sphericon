import { appRenderer } from './appRenderer.ts'
import { ExitEventsControl, GoalEventControl } from './EndEventControls.tsx'
import { TriggerControl } from './TriggerControl.tsx'
import { EXIT_EVENTS_FORMAT, GOAL_EVENT_FORMAT, TRIGGER_EVENT_FORMAT } from './triggerFormat.ts'

export const triggerRenderer = appRenderer(TRIGGER_EVENT_FORMAT, TriggerControl)
export const exitEventsRenderer = appRenderer(EXIT_EVENTS_FORMAT, ExitEventsControl)
export const goalEventRenderer = appRenderer(GOAL_EVENT_FORMAT, GoalEventControl)
