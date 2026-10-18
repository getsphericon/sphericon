import {
  type PaletteItem,
  sharedProperties,
  type UISchema,
  type WBIcon,
} from '@workflowbuilder/sdk'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

// The palette `type` of each node, stored on `node.data.type` and used by the
// graph ↔ definition converters to tell email steps from wait steps.
export const EMAIL_NODE_TYPE = 'email'
export const WAIT_NODE_TYPE = 'wait'
export const APPLY_TAG_NODE_TYPE = 'apply_tag'
export const REMOVE_TAG_NODE_TYPE = 'remove_tag'

// Phosphor icon names (WBIcon) the SDK renders in the palette and node header.
export const EMAIL_NODE_ICON = 'Envelope'
export const WAIT_NODE_ICON = 'Timer'
export const TAG_NODE_ICON: WBIcon = 'Tag'

// useAutomationNodeTypes builds the palette definitions for the visual builder.
// Memoized so the array keeps a stable reference across renders — the SDK
// overwrites its module-level palette holder whenever this reference changes.
export function useAutomationNodeTypes(): PaletteItem[] {
  const { t } = useTranslation()

  return useMemo(
    () => [
      {
        type: EMAIL_NODE_TYPE,
        icon: EMAIL_NODE_ICON,
        label: t(($) => $.automations.stepEmail),
        description: t(($) => $.automations.emailNodeDescription),
        defaultPropertiesData: { subject: '', body: '' },
        schema: {
          type: 'object',
          required: ['subject', 'body'],
          properties: {
            ...sharedProperties,
            subject: { type: 'string', label: t(($) => $.automations.emailSubjectLabel) },
            body: { type: 'string', label: t(($) => $.automations.emailBodyLabel) },
          },
        },
        uischema: {
          type: 'VerticalLayout',
          elements: [
            { type: 'Text', scope: '#/properties/subject', placeholder: '' },
            {
              type: 'TextArea',
              scope: '#/properties/body',
              minRows: 8,
              placeholder: t(($) => $.automations.emailBodyHint),
            },
          ],
        } satisfies UISchema,
      },
      {
        type: WAIT_NODE_TYPE,
        icon: WAIT_NODE_ICON,
        label: t(($) => $.automations.stepWait),
        description: t(($) => $.automations.waitNodeDescription),
        defaultPropertiesData: { seconds: 3600 },
        schema: {
          type: 'object',
          required: ['seconds'],
          properties: {
            ...sharedProperties,
            seconds: {
              type: 'number',
              label: t(($) => $.automations.waitSecondsLabel),
              minimum: 1,
            },
          },
        },
        uischema: {
          type: 'VerticalLayout',
          elements: [
            {
              type: 'Text',
              scope: '#/properties/seconds',
              inputType: 'number',
              placeholder: '3600',
            },
          ],
        } satisfies UISchema,
      },
      ...[
        {
          type: APPLY_TAG_NODE_TYPE,
          label: t(($) => $.automations.stepApplyTag),
          description: t(($) => $.automations.applyTagNodeDescription),
        },
        {
          type: REMOVE_TAG_NODE_TYPE,
          label: t(($) => $.automations.stepRemoveTag),
          description: t(($) => $.automations.removeTagNodeDescription),
        },
      ].map(({ type, label, description }) => ({
        type,
        icon: TAG_NODE_ICON,
        label,
        description,
        defaultPropertiesData: { tag: '' },
        schema: {
          type: 'object' as const,
          required: ['tag'],
          properties: {
            ...sharedProperties,
            tag: { type: 'string' as const, label: t(($) => $.automations.tagNameLabel) },
          },
        },
        uischema: {
          type: 'VerticalLayout',
          elements: [{ type: 'Text', scope: '#/properties/tag', placeholder: '' }],
        } satisfies UISchema,
      })),
    ],
    [t],
  )
}
