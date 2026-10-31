import { formatIs, rankWith, withJsonFormsControlProps } from '@workflowbuilder/sdk'
import type { ControlProps, JsonFormsRendererExtension } from '@workflowbuilder/sdk'
import { createElement } from 'react'
import type { ComponentType } from 'react'
import { I18nextProvider } from 'react-i18next'

import { i18n } from '../../i18n.ts'

// appRenderer renders the fields of the given format with one of the app's own controls. The
// builder subtree reads the global i18next instance (see AutomationBuilder); the control is the
// app's own UI, so it is bound back to the app's instance. Call it once per control, at module
// level: the SDK registers the renderers it is given, so each needs a stable reference.
export function appRenderer(
  format: string,
  control: ComponentType<ControlProps>,
): JsonFormsRendererExtension {
  const inApp = (props: ControlProps) =>
    createElement(I18nextProvider, { i18n }, createElement(control, props))
  return { tester: rankWith(100, formatIs(format)), renderer: withJsonFormsControlProps(inApp) }
}
