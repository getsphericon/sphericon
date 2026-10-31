import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { appRenderer } from './appRenderer.ts'
import { ExitEventsControl, GoalEventControl } from './EndEventControls.tsx'
import { TriggerControl } from './TriggerControl.tsx'
import { EXIT_EVENTS_FORMAT, GOAL_EVENT_FORMAT, TRIGGER_EVENT_FORMAT } from './triggerFormat.ts'

export const triggerRenderer: JsonFormsRendererExtension = appRenderer(
  TRIGGER_EVENT_FORMAT,
  TriggerControl,
)
export const exitEventsRenderer: JsonFormsRendererExtension = appRenderer(
  EXIT_EVENTS_FORMAT,
  ExitEventsControl,
)
export const goalEventRenderer: JsonFormsRendererExtension = appRenderer(
  GOAL_EVENT_FORMAT,
  GoalEventControl,
)
