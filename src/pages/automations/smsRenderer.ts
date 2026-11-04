import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { appRenderer } from './appRenderer.ts'
import { SMS_TEXT_FORMAT } from './smsFormat.ts'
import { SmsTextControl } from './SmsTextControl.tsx'

export const smsTextRenderer: JsonFormsRendererExtension = appRenderer(
  SMS_TEXT_FORMAT,
  SmsTextControl,
)
