// PROTOTYPE (throwaway, ticket #958). One JSON Schema per block type. The same schemas draw the
// inspector (RJSF), validate the whole stored document (as the server would at Publish) and are
// what an agent would be given over MCP (as ADR 0040 did for email).
import type { RJSFSchema, UiSchema } from '@rjsf/utils'

import type { BlockType } from './doc.ts'

export const BLOCK_SCHEMAS: Record<BlockType, RJSFSchema> = {
  heading: { type: 'object', required: ['text'], properties: { text: { type: 'string', title: 'Text', minLength: 1 }, size: { type: 'integer', title: 'Size (px)', minimum: 12, maximum: 64, default: 22 } } },
  text: { type: 'object', required: ['text'], properties: { text: { type: 'string', title: 'Text' } } },
  image: { type: 'object', required: ['src'], properties: { src: { type: 'string', title: 'Image URL', format: 'uri' }, alt: { type: 'string', title: 'Alt text' } } },
  email: { type: 'object', properties: { label: { type: 'string', title: 'Label', default: 'Email' }, placeholder: { type: 'string', title: 'Placeholder' }, required: { type: 'boolean', title: 'Required', default: true } } },
  'phone-sms': {
    type: 'object',
    required: ['keyword', 'number', 'disclosure'],
    properties: {
      keyword: { type: 'string', title: 'Keyword', default: 'START' },
      number: { type: 'string', title: 'Number the person texts', pattern: '^\\+[1-9][0-9]{6,14}$' },
      disclosure: { type: 'string', title: 'Disclosure shown beside the button', minLength: 20 },
    },
  },
  'push-button': { type: 'object', properties: { label: { type: 'string', title: 'Label' } } },
  button: {
    type: 'object',
    required: ['label', 'action'],
    properties: {
      label: { type: 'string', title: 'Label', minLength: 1 },
      action: { type: 'string', title: 'Action', enum: ['next', 'submit', 'close', 'url'] },
      url: { type: 'string', title: 'URL (action url)', format: 'uri' },
      variant: { type: 'string', title: 'Style', enum: ['solid', 'link'] },
    },
  },
  columns: { type: 'object', properties: {} },
}

export const BLOCK_UI: Partial<Record<BlockType, UiSchema>> = {
  text: { text: { 'ui:widget': 'textarea' } },
  'phone-sms': { disclosure: { 'ui:widget': 'textarea' } },
}

// Which props may differ on a phone; absent from the override = linked to desktop.
export const MOBILE_PROPS: Partial<Record<BlockType, string[]>> = { heading: ['text', 'size'], text: ['text'], image: ['src'], button: ['label'] }

export function mobileSchema(type: BlockType): RJSFSchema {
  const base = BLOCK_SCHEMAS[type].properties ?? {}
  const props = Object.fromEntries((MOBILE_PROPS[type] ?? []).map((k) => [k, { ...(base[k] as object), default: undefined }]))
  return {
    type: 'object',
    properties: {
      hidden: { type: 'boolean', title: 'Hide on phones' },
      props: { type: 'object', title: 'Phone overrides (empty = linked to desktop)', properties: props },
    },
  }
}

const blockSchema: RJSFSchema = {
  type: 'object',
  required: ['id', 'type', 'props'],
  properties: {
    id: { type: 'string' },
    type: { enum: Object.keys(BLOCK_SCHEMAS) },
    props: { type: 'object' },
    mobile: { type: 'object', properties: { hidden: { type: 'boolean' }, props: { type: 'object' } }, additionalProperties: false },
    left: { type: 'array', items: { $ref: '#/definitions/block' } },
    right: { type: 'array', items: { $ref: '#/definitions/block' } },
  },
  additionalProperties: false,
  // Each type's props must match its own schema.
  allOf: Object.entries(BLOCK_SCHEMAS).map(([t, s]) => ({ if: { properties: { type: { const: t } } }, then: { properties: { props: s } } })),
}

const stepSchema: RJSFSchema = { type: 'object', required: ['id', 'blocks'], properties: { id: { type: 'string' }, blocks: { type: 'array', items: { $ref: '#/definitions/block' } } }, additionalProperties: false }

// The closed schema of the whole stored document.
export const DOC_SCHEMA: RJSFSchema = {
  definitions: { block: blockSchema, step: stepSchema },
  type: 'object',
  required: ['schema', 'id', 'name', 'format', 'theme', 'steps'],
  additionalProperties: false,
  properties: {
    schema: { const: 1 },
    id: { type: 'string' },
    name: { type: 'string', minLength: 1 },
    format: { enum: ['popup', 'slide-in', 'bar', 'embedded'] },
    placement: { type: 'string' },
    theme: {
      type: 'object',
      required: ['primary', 'background', 'text', 'font', 'radius'],
      additionalProperties: false,
      properties: { primary: { type: 'string' }, background: { type: 'string' }, text: { type: 'string' }, font: { type: 'string' }, radius: { type: 'number', minimum: 0, maximum: 32 } },
    },
    steps: { type: 'array', minItems: 1, maxItems: 4, items: { $ref: '#/definitions/step' } },
    success: { $ref: '#/definitions/step' },
    teaser: { type: 'object', required: ['text', 'show'], properties: { text: { type: 'string' }, show: { enum: ['before', 'after', 'both'] } }, additionalProperties: false },
  },
}
