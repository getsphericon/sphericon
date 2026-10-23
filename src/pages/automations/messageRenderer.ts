import { formatIs, rankWith, withJsonFormsControlProps } from '@workflowbuilder/sdk'
import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { MessageControlInApp } from './MessageControl.tsx'
import { MESSAGE_FORMAT } from './messageFormat.ts'

// A stable reference: the SDK registers the renderers it is given.
export const messageRenderer: JsonFormsRendererExtension = {
  tester: rankWith(100, formatIs(MESSAGE_FORMAT)),
  renderer: withJsonFormsControlProps(MessageControlInApp),
}
