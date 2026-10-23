import { formatIs, rankWith, withJsonFormsControlProps } from '@workflowbuilder/sdk'
import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { TriggerControlInApp } from './TriggerControl.tsx'
import { TRIGGER_EVENT_FORMAT } from './triggerFormat.ts'

// A stable reference: the SDK registers the renderers it is given.
export const triggerRenderer: JsonFormsRendererExtension = {
  tester: rankWith(100, formatIs(TRIGGER_EVENT_FORMAT)),
  renderer: withJsonFormsControlProps(TriggerControlInApp),
}
