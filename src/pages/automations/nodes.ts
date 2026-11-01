import { NodeType, sharedProperties } from '@workflowbuilder/sdk'
import type { PaletteItem, UISchema, WBIcon } from '@workflowbuilder/sdk'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { HTTP_BODY_FORMAT, HTTP_HEADERS_FORMAT } from './httpFormat.ts'
import { MESSAGE_FORMAT } from './messageFormat.ts'
import { EXIT_EVENTS_FORMAT, GOAL_EVENT_FORMAT, TRIGGER_EVENT_FORMAT } from './triggerFormat.ts'

// The palette `type` of each node, stored on `node.data.type` and used by the
// graph ↔ definition converters to tell email steps from wait steps.
export const EMAIL_NODE_TYPE = 'email'
export const WAIT_NODE_TYPE = 'wait'
export const APPLY_TAG_NODE_TYPE = 'apply_tag'
export const REMOVE_TAG_NODE_TYPE = 'remove_tag'
export const HTTP_NODE_TYPE = 'http'
// The Trigger is not a Step: it is the canvas's start node and is saved as the automation's
// trigger event, never in the steps.
export const TRIGGER_NODE_TYPE = 'trigger'

// Phosphor icon names (WBIcon) the SDK renders in the palette and node header.
export const EMAIL_NODE_ICON = 'Envelope'
export const WAIT_NODE_ICON = 'Timer'
export const TAG_NODE_ICON: WBIcon = 'Tag'
export const HTTP_NODE_ICON: WBIcon = 'WebhooksLogo'
export const TRIGGER_NODE_ICON: WBIcon = 'Lightning'

export const TRIGGER_EVENTS = [
  'contact.created',
  'contact.tagged',
  'contact.untagged',
  'contact.field_changed',
  'email.opened',
  'email.clicked',
  'email.unsubscribed',
  'automation.enrolled',
  'automation.completed',
  'automation.goal_reached',
  'automation.exited',
] as const
export const DEFAULT_TRIGGER_EVENT = TRIGGER_EVENTS[0]

// useAutomationNodeTypes builds the palette definitions for the visual builder.
// Memoized so the array keeps a stable reference across renders — the SDK
// overwrites its module-level palette holder whenever this reference changes.
export function useAutomationNodeTypes(): PaletteItem[] {
  const { t } = useTranslation()

  return useMemo(
    () => [
      {
        type: TRIGGER_NODE_TYPE,
        icon: TRIGGER_NODE_ICON,
        templateType: NodeType.StartNode,
        isStartNode: true,
        label: t(($) => $.automations.stepTrigger),
        description: t(($) => $.automations.triggerHint),
        // The exit events are held one per line and the goal event as text ('' for none): the
        // SDK's schema has no list of strings.
        defaultPropertiesData: {
          triggerEvent: DEFAULT_TRIGGER_EVENT,
          exitEvents: '',
          goalEvent: '',
        },
        schema: {
          type: 'object',
          required: ['triggerEvent'],
          properties: {
            ...sharedProperties,
            triggerEvent: {
              type: 'string',
              label: t(($) => $.automations.triggerLabel),
              format: TRIGGER_EVENT_FORMAT,
            },
            exitEvents: {
              type: 'string',
              label: t(($) => $.automations.exitEventsLabel),
              format: EXIT_EVENTS_FORMAT,
            },
            goalEvent: {
              type: 'string',
              label: t(($) => $.automations.goalEventLabel),
              format: GOAL_EVENT_FORMAT,
            },
          },
        },
        uischema: {
          type: 'VerticalLayout',
          elements: [
            { type: 'Text', scope: '#/properties/triggerEvent', placeholder: '' },
            { type: 'Text', scope: '#/properties/exitEvents', placeholder: '' },
            { type: 'Text', scope: '#/properties/goalEvent', placeholder: '' },
          ],
        } satisfies UISchema,
      },
      {
        type: EMAIL_NODE_TYPE,
        icon: EMAIL_NODE_ICON,
        label: t(($) => $.automations.stepEmail),
        description: t(($) => $.automations.emailNodeDescription),
        // `doc` is the editor's document as JSON text ('' for a step written as HTML); the
        // message control edits it and the body together.
        defaultPropertiesData: { subject: '', body: '', doc: '' },
        schema: {
          type: 'object',
          required: ['subject'],
          properties: {
            ...sharedProperties,
            subject: { type: 'string', label: t(($) => $.automations.emailSubjectLabel) },
            body: {
              type: 'string',
              format: MESSAGE_FORMAT,
              label: t(($) => $.automations.emailBodyLabel),
            },
            doc: { type: 'string' },
          },
        },
        uischema: {
          type: 'VerticalLayout',
          elements: [
            {
              type: 'Text',
              scope: '#/properties/subject',
              label: t(($) => $.automations.emailSubjectLabel),
              placeholder: '',
            },
            {
              type: 'Text',
              scope: '#/properties/body',
              label: t(($) => $.automations.emailBodyLabel),
              placeholder: '',
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
              label: t(($) => $.automations.waitSecondsLabel),
              inputType: 'number',
              placeholder: '3600',
            },
          ],
        } satisfies UISchema,
      },
      {
        type: HTTP_NODE_TYPE,
        icon: HTTP_NODE_ICON,
        label: t(($) => $.automations.stepHttp),
        description: t(($) => $.automations.httpNodeDescription),
        defaultPropertiesData: { stepId: '', url: '', headers: '[]', body: '{}', secret: '' },
        schema: {
          type: 'object',
          required: ['body'],
          properties: {
            ...sharedProperties,
            url: { type: 'string', label: t(($) => $.automations.httpUrlLabel) },
            headers: {
              type: 'string',
              label: t(($) => $.automations.httpHeadersLabel),
              format: HTTP_HEADERS_FORMAT,
            },
            body: {
              type: 'string',
              label: t(($) => $.automations.httpBodyLabel),
              format: HTTP_BODY_FORMAT,
            },
            secret: { type: 'string', label: t(($) => $.automations.httpSecretLabel) },
          },
        },
        uischema: {
          type: 'VerticalLayout',
          elements: [
            {
              type: 'Text',
              scope: '#/properties/url',
              label: t(($) => $.automations.httpUrlLabel),
              placeholder: t(($) => $.automations.httpUrlHint),
            },
            {
              type: 'Text',
              scope: '#/properties/headers',
              label: t(($) => $.automations.httpHeadersLabel),
              placeholder: '',
            },
            {
              type: 'Text',
              scope: '#/properties/body',
              label: t(($) => $.automations.httpBodyLabel),
              placeholder: '',
            },
            {
              type: 'Text',
              scope: '#/properties/secret',
              label: t(($) => $.automations.httpSecretLabel),
              placeholder: t(($) => $.automations.httpSecretHint),
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
          elements: [
            {
              type: 'Text',
              scope: '#/properties/tag',
              label: t(($) => $.automations.tagNameLabel),
              placeholder: '',
            },
          ],
        } satisfies UISchema,
      })),
    ],
    [t],
  )
}
