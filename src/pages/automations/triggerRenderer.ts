import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { appRenderer } from './appRenderer.ts'
import { ExitEventsControl, GoalEventControl } from './EndEventControls.tsx'
import { CooldownControl, EntryPolicyControl } from './EntryControls.tsx'
import { TriggerControl } from './TriggerControl.tsx'
import { TriggerFilterControl } from './TriggerFilterControl.tsx'
import {
  COOLDOWN_FORMAT,
  ENTRY_POLICY_FORMAT,
  EXIT_EVENTS_FORMAT,
  GOAL_EVENT_FORMAT,
  TRIGGER_EVENT_FORMAT,
  TRIGGER_FILTER_FORMAT,
} from './triggerFormat.ts'

export const triggerRenderer: JsonFormsRendererExtension = appRenderer(
  TRIGGER_EVENT_FORMAT,
  TriggerControl,
)
export const triggerFilterRenderer: JsonFormsRendererExtension = appRenderer(
  TRIGGER_FILTER_FORMAT,
  TriggerFilterControl,
)
export const exitEventsRenderer: JsonFormsRendererExtension = appRenderer(
  EXIT_EVENTS_FORMAT,
  ExitEventsControl,
)
export const goalEventRenderer: JsonFormsRendererExtension = appRenderer(
  GOAL_EVENT_FORMAT,
  GoalEventControl,
)
export const entryPolicyRenderer: JsonFormsRendererExtension = appRenderer(
  ENTRY_POLICY_FORMAT,
  EntryPolicyControl,
)
export const cooldownRenderer: JsonFormsRendererExtension = appRenderer(
  COOLDOWN_FORMAT,
  CooldownControl,
)
