import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { appRenderer } from './appRenderer.ts'
import { MessageControl } from './MessageControl.tsx'
import { MESSAGE_FORMAT } from './messageFormat.ts'

export const messageRenderer: JsonFormsRendererExtension = appRenderer(
  MESSAGE_FORMAT,
  MessageControl,
)
