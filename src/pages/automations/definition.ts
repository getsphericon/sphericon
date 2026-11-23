import { NodeType } from '@workflowbuilder/sdk'
import type { WBIcon, WorkflowBuilderEdge, WorkflowBuilderNode } from '@workflowbuilder/sdk'
import * as z from 'zod'

import type { AnyCondition, ConditionDraft } from '../../components/conditions/condition.ts'
import type {
  AutomationHttpHeader,
  AutomationPath,
  AutomationPathInput,
  AutomationStep,
  AutomationStepInput,
  AutomationTriggerInput,
  AutomationVersion,
  EntryPolicy,
  EventFilterInput,
  TelegramStep,
} from '../../generated/site/types.gen.ts'
import {
  zAutomationHttpHeader,
  zEntryPolicy,
  zEventFilterInput,
} from '../../generated/site/zod.gen.ts'
import { jsonText } from '../../utils/jsonText.ts'
import { docCodec } from '../../utils/messageDoc.ts'
import { buttonsCodec } from '../../utils/telegramButtons.ts'
import { completeButtons } from '../../utils/telegramContent.ts'
import { OFF, STORED } from './nodeProperties.ts'
import type { NodeProperties, ReadProperties, StepKind } from './nodeProperties.ts'
import {
  EMAIL_NODE_ICON,
  END_NODE_ICON,
  HTTP_NODE_ICON,
  SMS_NODE_ICON,
  TAG_NODE_ICON,
  TELEGRAM_NODE_ICON,
  TRIGGER_NODE_ICON,
  TRIGGER_NODE_TYPE,
  WAIT_NODE_ICON,
} from './nodes.ts'

// A step is the generated contract type (typed in TypeSpec; no hand-rolled
// parse/serialize — the API carries steps as structured data, not a JSON string): a union by
// `type`, each kind with fields of its own. A version's Steps are its root path, {id, steps}.
export type { AutomationStep }

// --- graph ↔ steps (the visual builder uses an xyflow graph; we persist the root path) ---
// A Step's node has the Step's id as its id (ADR 0122): a loaded node takes the stored id, and a
// node dropped on the canvas the UUID the SDK mints for it, so every save sends each Step with the
// id it keeps, and an HTTP step keeps its secrets by it. The root path's id rides on the Trigger
// node (pathId).

// The SDK's default node renderer key (NodeType.Node) and default edge type.
const NODE_RENDERER_TYPE = 'node'
const EDGE_TYPE = 'labelEdge'
// Default node handle ids the SDK's node template uses for a single in/out port.
const SOURCE_HANDLE = 'source'
const TARGET_HANDLE = 'target'
// Vertical spacing for the synthesized chain (layout is not persisted — we lay
// the chain out top-to-bottom on load and let the user rearrange freely).
const Y_GAP = 160

export interface AutomationGraph {
  nodes: WorkflowBuilderNode[]
  edges: WorkflowBuilderEdge[]
}

const STEP_ICON: Record<StepKind, WBIcon> = {
  email: EMAIL_NODE_ICON,
  telegram: TELEGRAM_NODE_ICON,
  sms: SMS_NODE_ICON,
  wait: WAIT_NODE_ICON,
  apply_tag: TAG_NODE_ICON,
  remove_tag: TAG_NODE_ICON,
  http: HTTP_NODE_ICON,
  end: END_NODE_ICON,
}

// A Telegram step's content, as the form holds it: every value a string.
function telegramProperties(step: TelegramStep): NodeProperties['telegram'] {
  return {
    integrationId: step.integrationId ?? '',
    text: step.text,
    unsubscribeButton: step.unsubscribeButton ?? '',
    imageUrl: step.imageUrl ?? '',
    buttons: buttonsCodec.write(step.buttons),
    linkPreview: step.linkPreview ? '' : OFF,
  }
}

// The server never returns an HTTP step's URL, header values or signing secret, so the
// canvas holds them blank: a blank field is "keep what is stored". The headers come back as
// names only, held as JSON text (headersText) for the same reason.
function stepProperties(step: AutomationStep): NodeProperties[keyof NodeProperties] {
  switch (step.type) {
    case 'http': {
      const http: NodeProperties['http'] = {
        stored: STORED,
        url: '',
        headers: headersText(step.headers.map(({ name }) => ({ name }))),
        body: step.body,
        secret: '',
      }
      return http
    }
    case 'wait': {
      const wait: NodeProperties['wait'] = { seconds: step.seconds }
      return wait
    }
    case 'apply_tag':
    case 'remove_tag': {
      const tag: NodeProperties['tag'] = { tag: step.tag }
      return tag
    }
    case 'telegram':
      return telegramProperties(step)
    case 'end': {
      const end: NodeProperties['end'] = {}
      return end
    }
    case 'sms': {
      const sms: NodeProperties['sms'] = {
        integrationId: step.integrationId ?? '',
        text: step.text,
        linkTracking: step.linkTracking ? '' : OFF,
      }
      return sms
    }
    case 'email':
    default: {
      const email: NodeProperties['email'] = {
        integrationId: step.integrationId ?? '',
        subject: step.subject,
        body: step.body,
        doc: step.doc ? z.encode(docCodec, step.doc) : '',
      }
      return email
    }
  }
}

const TRIGGER_NODE_ID = 'trigger'

// What the Trigger node holds: the Event matcher that enrolls a Contact (ADR 0133), the events that
// end an Enrollment early and the one that ends it as having reached its goal (#751).
export type AutomationTrigger = Pick<AutomationVersion, 'trigger' | 'exitEvents' | 'goalEvent'>

// The canvas holds the Trigger's action as text and its filter as JSON text ('' for none), its
// Entry policy and its cooldown as text of seconds (ADR 0127), the exit events as one line each, the goal event
// as text ('' for none), and the id of the root path the steps hang from.
function triggerProperties(trigger: AutomationTrigger, pathId: string): Record<string, unknown> {
  return {
    triggerEvent: trigger.trigger.action,
    triggerFilter: trigger.trigger.filter ? filterText(trigger.trigger.filter) : '',
    entryPolicy: trigger.trigger.entryPolicy,
    cooldown: String(trigger.trigger.cooldown),
    exitEvents: trigger.exitEvents.join('\n'),
    goalEvent: trigger.goalEvent ?? '',
    pathId,
  }
}

// filterText is the Trigger node's text of a filter as the builder edits it (complete or not):
// '' for an empty group, which tests nothing and is no filter, else its JSON.
export function filterText(filter: AnyCondition | ConditionDraft): string {
  return filter.entries.length === 0 && filter.not !== true ? '' : JSON.stringify(filter)
}

// The filter the Trigger node's text holds, as the builder loads it: a complete filter, or none
// for '' and for a filter still being typed, which the builder then starts over.
export function filterOf(text: string): EventFilterInput | undefined {
  if (text === '') {
    return undefined
  }
  const parsed = jsonText.pipe(zEventFilterInput).safeParse(text)
  return parsed.success ? parsed.data : undefined
}

// entryPolicyOf reads an Entry policy off a node's value: `once` for anything that is none.
export function entryPolicyOf(value: unknown): EntryPolicy {
  const parsed = zEntryPolicy.safeParse(value)
  return parsed.success ? parsed.data : 'once'
}

// cooldownHours shows a cooldown the node holds as text of seconds in hours, '' while none is set.
export function cooldownHours(value: unknown): number | '' {
  const seconds = cooldownOf(value)
  return seconds === undefined ? '' : seconds / 3600
}

// cooldownSeconds turns the hours a person typed into the text of seconds the node holds, '' for
// none.
export function cooldownSeconds(hours: number | string): string {
  return typeof hours === 'number' ? String(hours * 3600) : ''
}

// cooldownOf reads the cooldown in seconds off the node's text, none when it holds no number.
function cooldownOf(value: unknown): number | undefined {
  const text = asText(value).trim()
  const seconds = Number(text)
  return text === '' || !Number.isInteger(seconds) ? undefined : seconds
}

// triggerOf reads the Trigger off the node: its action, its Entry policy and cooldown (a cooldown
// left empty is the server's default) and, when the filter's text holds a complete filter, that
// filter. incomplete says the text holds a filter still being typed (a blank key or number), which
// is no filter to save.
function triggerOf(properties: Record<string, unknown>): {
  trigger: AutomationTriggerInput
  incomplete: boolean
} {
  const trigger: AutomationTriggerInput = {
    kind: 'event',
    action: asText(properties.triggerEvent),
    entryPolicy: entryPolicyOf(properties.entryPolicy),
  }
  const cooldown = cooldownOf(properties.cooldown)
  if (cooldown !== undefined) {
    trigger.cooldown = cooldown
  }
  const text = asText(properties.triggerFilter)
  if (text === '') {
    return { trigger, incomplete: false }
  }
  const parsed = jsonText.pipe(zEventFilterInput).safeParse(text)
  if (!parsed.success) {
    return { trigger, incomplete: true }
  }
  return { trigger: { ...trigger, filter: parsed.data }, incomplete: false }
}

// exitEventsOf reads the exit events off the Trigger node's lines, dropping blank ones.
export function exitEventsOf(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')
}

function labelOf(labels: Record<string, string>, type: string): { label?: string } {
  const label = labels[type]
  return label === undefined ? {} : { label }
}

// stepsToGraph builds a linear top-to-bottom chain for the canvas: the Trigger as the start
// node, holding the root path's id, then one node per stored step, its id the Step's. Positions
// are synthesized from order, never persisted; `labels` names each node kind on the canvas.
export function stepsToGraph(
  trigger: AutomationTrigger,
  path: AutomationPath,
  labels: Record<string, string>,
): AutomationGraph {
  const start: WorkflowBuilderNode = {
    id: TRIGGER_NODE_ID,
    type: NODE_RENDERER_TYPE,
    position: { x: 0, y: 0 },
    deletable: false,
    data: {
      type: TRIGGER_NODE_TYPE,
      icon: TRIGGER_NODE_ICON,
      templateType: NodeType.StartNode,
      isStartNode: true,
      segments: [],
      properties: {
        ...triggerProperties(trigger, path.id),
        ...labelOf(labels, TRIGGER_NODE_TYPE),
      },
    },
  }
  const nodes: WorkflowBuilderNode[] = [
    start,
    ...path.steps.map((step, i): WorkflowBuilderNode => ({
      id: step.id,
      type: NODE_RENDERER_TYPE,
      position: { x: 0, y: (i + 1) * Y_GAP },
      data: {
        type: step.type,
        icon: STEP_ICON[step.type],
        segments: [],
        properties: { ...stepProperties(step), ...labelOf(labels, step.type) },
      },
    })),
  ]

  const edges: WorkflowBuilderEdge[] = nodes.slice(1).map((node, i) => ({
    id: `edge-${i}`,
    source: nodes[i]?.id ?? TRIGGER_NODE_ID,
    target: node.id,
    sourceHandle: SOURCE_HANDLE,
    targetHandle: TARGET_HANDLE,
    type: EDGE_TYPE,
  }))

  return { nodes, edges }
}

function asText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

// An HTTP step's headers as the node holds them: JSON text (see jsonText).
const headersCodec = jsonText.pipe(z.array(zAutomationHttpHeader))

// headersOf reads an HTTP step's headers off the node; a blank or unreadable text is none.
export function headersOf(text: string): AutomationHttpHeader[] {
  const parsed = headersCodec.safeParse(text)
  return parsed.success ? parsed.data : []
}

// headersText is the JSON text a node holds its headers in.
export function headersText(headers: AutomationHttpHeader[]): string {
  return z.encode(headersCodec, headers)
}

// stepHeaders are the headers a step saves: a row left wholly blank is dropped, and a header
// whose value is empty keeps the stored value of the header with that name.
function stepHeaders(text: string): AutomationHttpHeader[] {
  return headersOf(text)
    .map(({ name, value }) => ({ name: name.trim(), value: value ?? '' }))
    .filter(({ name, value }) => name !== '' || value !== '')
    .map(({ name, value }) => (value === '' ? { name } : { name, value }))
}

// The Telegram step a node holds: what is left blank is left out.
function telegramStep(id: string, props: ReadProperties<'telegram'>): AutomationStepInput {
  const integrationId = asText(props.integrationId)
  const unsubscribeButton = asText(props.unsubscribeButton)
  const imageUrl = asText(props.imageUrl).trim()
  const buttons = completeButtons(buttonsCodec.read(props.buttons))
  return {
    id,
    type: 'telegram',
    text: asText(props.text),
    ...(integrationId === '' ? {} : { integrationId }),
    ...(unsubscribeButton === '' ? {} : { unsubscribeButton }),
    ...(imageUrl === '' ? {} : { imageUrl }),
    ...(buttons.length === 0 ? {} : { buttons }),
    ...(props.linkPreview === OFF ? { linkPreview: false } : {}),
  }
}

// The SMS step a node holds. A step that names no Integration ('') leaves it out: it sends
// through the SMS default. Link tracking is on unless the node turned it 'off'.
function smsStep(id: string, props: ReadProperties<'sms'>): AutomationStepInput {
  const integrationId = asText(props.integrationId)
  return {
    id,
    type: 'sms',
    text: asText(props.text),
    ...(integrationId === '' ? {} : { integrationId }),
    ...(props.linkTracking === OFF ? { linkTracking: false } : {}),
  }
}

// The Step a node holds, with the node's id as its own; null for a node that is no Step.
function nodeToStep(node: WorkflowBuilderNode): AutomationStepInput | null {
  const props: Record<string, unknown> = node.data.properties
  const { id } = node
  switch (node.data.type) {
    case 'http': {
      const http: ReadProperties<'http'> = props
      const url = asText(http.url).trim()
      const secret = asText(http.secret)
      return {
        id,
        type: 'http',
        body: asText(http.body),
        headers: stepHeaders(asText(http.headers)),
        ...(url === '' ? {} : { url }),
        ...(secret === '' ? {} : { secret }),
      }
    }
    case 'wait': {
      const wait: ReadProperties<'wait'> = props
      return { id, type: 'wait', seconds: Math.trunc(Number(wait.seconds)) || 0 }
    }
    case 'apply_tag':
    case 'remove_tag': {
      const tag: ReadProperties<'tag'> = props
      return { id, type: node.data.type, tag: asText(tag.tag) }
    }
    case 'telegram':
      return telegramStep(id, props)
    case 'sms':
      return smsStep(id, props)
    // An End holds nothing but its id: the canvas cannot place one, but keeps one it read.
    case 'end':
      return { id, type: 'end' }
    case 'email': {
      // A message written in the editor is saved as its document alone (the server builds the
      // body; sending both is a 422), and one written as HTML as its body alone. A step that names
      // no Integration ('') leaves it out: it sends through the email default.
      const email: ReadProperties<'email'> = props
      const subject = asText(email.subject)
      const integrationId = asText(email.integrationId)
      const via = integrationId === '' ? {} : { integrationId }
      const doc = docCodec.safeParse(asText(email.doc))
      return doc.success
        ? { id, type: 'email', ...via, subject, doc: doc.data }
        : { id, type: 'email', ...via, subject, body: asText(email.body) }
    }
    default:
      return null
  }
}

// graphToSteps linearizes the canvas graph: it finds the one Trigger node, reads the trigger,
// exit and goal events and the root path's id off it and walks single outgoing edges from there
// into the root path's ordered steps, each with its node's id.
// The Trigger is not a Step. Branches are not supported yet: nodes off the main chain are
// reported via `dropped` so the caller can warn instead of silently losing them. `trigger` is
// null when the canvas does not hold exactly one Trigger; `filterIncomplete` says its filter is
// still being typed, and is left out of `trigger`.
export function graphToSteps(graph: AutomationGraph): {
  trigger: AutomationTriggerInput | null
  filterIncomplete: boolean
  exitEvents: string[]
  goalEvent: string | null
  steps: AutomationPathInput
  dropped: number
} {
  const ordered = linearGraphNodes(graph)
  const [root] = ordered
  if (!root) {
    return {
      trigger: null,
      filterIncomplete: false,
      exitEvents: [],
      goalEvent: null,
      steps: { steps: [] },
      dropped: 0,
    }
  }
  const steps = ordered.flatMap((node) => {
    const step = nodeToStep(node)
    return step ? [step] : []
  })
  const goalEvent = asText(root.data.properties.goalEvent).trim()
  const pathId = asText(root.data.properties.pathId)
  const { trigger, incomplete } = triggerOf(root.data.properties)
  return {
    trigger,
    filterIncomplete: incomplete,
    exitEvents: exitEventsOf(asText(root.data.properties.exitEvents)),
    goalEvent: goalEvent === '' ? null : goalEvent,
    steps: pathId === '' ? { steps } : { id: pathId, steps },
    dropped: graph.nodes.length - ordered.length,
  }
}

// The canvas order is its edges, not the order in which nodes were created.
function linearGraphNodes(graph: AutomationGraph): WorkflowBuilderNode[] {
  const { nodes, edges } = graph
  const triggers = nodes.filter((n) => n.data.type === TRIGGER_NODE_TYPE)
  const [root] = triggers
  if (triggers.length !== 1 || !root) {
    return []
  }

  const byId = new Map(nodes.map((n) => [n.id, n]))
  const next = new Map<string, string>()
  for (const e of edges) {
    if (byId.has(e.source) && byId.has(e.target) && !next.has(e.source)) {
      next.set(e.source, e.target)
    }
  }

  const ordered: WorkflowBuilderNode[] = []
  const visited = new Set<string>()
  let current: string | undefined = root.id
  while (current && !visited.has(current)) {
    visited.add(current)
    const node = byId.get(current)
    if (node) {
      ordered.push(node)
    }
    current = next.get(current)
  }

  return ordered
}
