import type { JsonFormsRendererExtension } from '@workflowbuilder/sdk'

import { appRenderer } from './appRenderer.ts'
import { HTTP_BODY_FORMAT, HTTP_HEADERS_FORMAT } from './httpFormat.ts'
import { HttpBodyControl, HttpHeadersControl } from './HttpStepControls.tsx'

export const httpHeadersRenderer: JsonFormsRendererExtension = appRenderer(
  HTTP_HEADERS_FORMAT,
  HttpHeadersControl,
)
export const httpBodyRenderer: JsonFormsRendererExtension = appRenderer(
  HTTP_BODY_FORMAT,
  HttpBodyControl,
)
