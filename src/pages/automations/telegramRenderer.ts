import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { appRenderer } from './appRenderer.ts'
import { TELEGRAM_TEXT_FORMAT } from './telegramFormat.ts'
import { TelegramTextControl } from './TelegramTextControl.tsx'

export const telegramTextRenderer: JsonFormsRendererExtension = appRenderer(
  TELEGRAM_TEXT_FORMAT,
  TelegramTextControl,
)
