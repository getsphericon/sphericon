import { Fieldset } from '@mantine/core'
import { formatIs, rankWith, withJsonFormsControlProps } from '@workflowbuilder/sdk'
import type { ControlProps, JsonFormsRendererExtension } from '@workflowbuilder/sdk'
import { createElement } from 'react'
import type { ComponentType } from 'react'

// appRenderer renders the fields of the given format with one of the app's own controls. Call it
// once per control, at module level: the SDK registers the renderers it is given, so each needs a
// stable reference.
export function appRenderer(
  format: string,
  control: ComponentType<ControlProps>,
): JsonFormsRendererExtension {
  const field = (props: ControlProps) =>
    createElement(
      Fieldset,
      // Convey disabled without native fieldset spacing changing the properties panel's size.
      { disabled: !props.enabled, variant: 'unstyled', m: 0, miw: 0 },
      createElement(control, props),
    )
  return { tester: rankWith(100, formatIs(format)), renderer: withJsonFormsControlProps(field) }
}
