import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { appRenderer } from './appRenderer.ts'
import { integrationControl } from './IntegrationControl.tsx'
import { integrationFormat } from './integrationFormat.ts'

export const emailIntegrationRenderer: JsonFormsRendererExtension = appRenderer(
  integrationFormat('email'),
  integrationControl('email'),
)

export const telegramIntegrationRenderer: JsonFormsRendererExtension = appRenderer(
  integrationFormat('telegram'),
  integrationControl('telegram'),
)
