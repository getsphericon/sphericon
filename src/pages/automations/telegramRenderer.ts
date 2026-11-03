import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { appRenderer } from './appRenderer.ts'
import { TELEGRAM_TEXT_FORMAT, TELEGRAM_UNSUBSCRIBE_FORMAT } from './telegramFormat.ts'
import { TelegramTextControl } from './TelegramTextControl.tsx'
import { TelegramUnsubscribeControl } from './TelegramUnsubscribeControl.tsx'

export const telegramTextRenderer: JsonFormsRendererExtension = appRenderer(
  TELEGRAM_TEXT_FORMAT,
  TelegramTextControl,
)

export const telegramUnsubscribeRenderer: JsonFormsRendererExtension = appRenderer(
  TELEGRAM_UNSUBSCRIBE_FORMAT,
  TelegramUnsubscribeControl,
)
