import { NodeType } from '@workflowbuilder/sdk'
import type { WBIcon, WorkflowBuilderEdge, WorkflowBuilderNode } from '@workflowbuilder/sdk'
import * as z from 'zod'

import { SiteAutomationStepType } from '../../generated/site/types.gen.ts'
import type {
  SiteAutomationHttpHeader,
  SiteAutomationStep,
  SiteAutomationVersion,
} from '../../generated/site/types.gen.ts'
import { zSiteAutomationHttpHeader } from '../../generated/site/zod.gen.ts'
import { jsonText } from '../../utils/jsonText.ts'
import { docCodec } from '../../utils/messageDoc.ts'
import { buttonsCodec } from '../../utils/telegramButtons.ts'
import { completeButtons } from '../../utils/telegramContent.ts'
import { OFF } from './nodeProperties.ts'
import type { NodeProperties, ReadProperties } from './nodeProperties.ts'
import {
  EMAIL_NODE_ICON,
  HTTP_NODE_ICON,
  SMS_NODE_ICON,
  TAG_NODE_ICON,
  TELEGRAM_NODE_ICON,
  TRIGGER_NODE_ICON,
  TRIGGER_NODE_TYPE,
  WAIT_NODE_ICON,
} from './nodes.ts'

// A step is the generated contract type (typed in TypeSpec; no hand-rolled
// parse/serialize — the API carries steps as structured data, not a JSON string).
export type AutomationStep = SiteAutomationStep

// --- graph ↔ steps (the visual builder uses an xyflow graph; we persist []step) ---

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

const STEP_ICON: Record<AutomationStep['type'], WBIcon> = {
  email: EMAIL_NODE_ICON,
  telegram: TELEGRAM_NODE_ICON,
  sms: SMS_NODE_ICON,
  wait: WAIT_NODE_ICON,
  apply_tag: TAG_NODE_ICON,
  remove_tag: TAG_NODE_ICON,
  http: HTTP_NODE_ICON,
}

// A Telegram step's content, as the form holds it: every value a string.
function telegramProperties(step: AutomationStep): NodeProperties['telegram'] {
  return {
    integrationId: step.integrationId ?? '',
    text: step.text ?? '',
    unsubscribeButton: step.unsubscribeButton ?? '',
    imageUrl: step.imageUrl ?? '',
    buttons: buttonsCodec.write(step.buttons ?? []),
    linkPreview: step.linkPreview === false ? OFF : '',
  }
}

// The server never returns an HTTP step's URL, header values or signing secret, so the
// canvas holds them blank: a blank field is "keep what is stored". The headers come back as
// names only, held as JSON text (headersText) for the same reason.
function stepProperties(step: AutomationStep): NodeProperties[keyof NodeProperties] {
  if (step.type === 'http') {
    const http: NodeProperties['http'] = {
      stepId: step.id ?? '',
      url: '',
      headers: headersText((step.headers ?? []).map(({ name }) => ({ name }))),
      body: step.body ?? '',
      secret: '',
    }
    return http
  }
  if (step.type === 'wait') {
    const wait: NodeProperties['wait'] = { seconds: step.seconds ?? 0 }
    return wait
  }
  if (step.type === 'apply_tag' || step.type === 'remove_tag') {
    const tag: NodeProperties['tag'] = { tag: step.tag ?? '' }
    return tag
  }
  if (step.type === 'telegram') {
    return telegramProperties(step)
  }
  if (step.type === 'sms') {
    const sms: NodeProperties['sms'] = {
      integrationId: step.integrationId ?? '',
      text: step.text ?? '',
      linkTracking: step.linkTracking === false ? OFF : '',
    }
    return sms
  }
  const email: NodeProperties['email'] = {
    integrationId: step.integrationId ?? '',
    subject: step.subject ?? '',
    body: step.body ?? '',
    doc: step.doc ? z.encode(docCodec, step.doc) : '',
  }
  return email
}

const TRIGGER_NODE_ID = 'trigger'

// What the Trigger node holds: the event that enrolls a Contact, the events that end an
// Enrollment early and the one that ends it as having reached its goal (#751).
export type AutomationTrigger = Pick<
  SiteAutomationVersion,
  'triggerEvent' | 'exitEvents' | 'goalEvent'
>

// The canvas holds the exit events as one line each, the goal event as text ('' for none).
function triggerProperties(trigger: AutomationTrigger): Record<string, unknown> {
  return {
    triggerEvent: trigger.triggerEvent,
    exitEvents: trigger.exitEvents.join('\n'),
    goalEvent: trigger.goalEvent ?? '',
  }
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
// node, then one node per stored step. Positions are synthesized from order, never persisted;
// `labels` names each node kind on the canvas.
export function stepsToGraph(
  trigger: AutomationTrigger,
  steps: AutomationStep[],
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
      properties: { ...triggerProperties(trigger), ...labelOf(labels, TRIGGER_NODE_TYPE) },
    },
  }
  const nodes: WorkflowBuilderNode[] = [
    start,
    ...steps.map((step, i): WorkflowBuilderNode => ({
      id: `step-${i}`,
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
const headersCodec = jsonText.pipe(z.array(zSiteAutomationHttpHeader))

// headersOf reads an HTTP step's headers off the node; a blank or unreadable text is none.
export function headersOf(text: string): SiteAutomationHttpHeader[] {
  const parsed = headersCodec.safeParse(text)
  return parsed.success ? parsed.data : []
}

// headersText is the JSON text a node holds its headers in.
export function headersText(headers: SiteAutomationHttpHeader[]): string {
  return z.encode(headersCodec, headers)
}

// stepHeaders are the headers a step saves: a row left wholly blank is dropped, and a header
// whose value is empty keeps the stored value of the header with that name.
function stepHeaders(text: string): SiteAutomationHttpHeader[] {
  return headersOf(text)
    .map(({ name, value }) => ({ name: name.trim(), value: value ?? '' }))
    .filter(({ name, value }) => name !== '' || value !== '')
    .map(({ name, value }) => (value === '' ? { name } : { name, value }))
}

// The Telegram step a node holds: what is left blank is left out.
function telegramStep(props: ReadProperties<'telegram'>): AutomationStep {
  const integrationId = asText(props.integrationId)
  const unsubscribeButton = asText(props.unsubscribeButton)
  const imageUrl = asText(props.imageUrl).trim()
  const buttons = completeButtons(buttonsCodec.read(props.buttons))
  return {
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
function smsStep(props: ReadProperties<'sms'>): AutomationStep {
  const integrationId = asText(props.integrationId)
  return {
    type: 'sms',
    text: asText(props.text),
    ...(integrationId === '' ? {} : { integrationId }),
    ...(props.linkTracking === OFF ? { linkTracking: false } : {}),
  }
}

function nodeToStep(node: WorkflowBuilderNode): AutomationStep | null {
  const props: Record<string, unknown> = node.data.properties
  if (node.data.type === SiteAutomationStepType.HTTP) {
    const http: ReadProperties<'http'> = props
    const id = asText(http.stepId)
    const url = asText(http.url).trim()
    const secret = asText(http.secret)
    return {
      type: 'http',
      body: asText(http.body),
      headers: stepHeaders(asText(http.headers)),
      ...(id === '' ? {} : { id }),
      ...(url === '' ? {} : { url }),
      ...(secret === '' ? {} : { secret }),
    }
  }
  if (node.data.type === SiteAutomationStepType.WAIT) {
    const wait: ReadProperties<'wait'> = props
    return { type: 'wait', seconds: Math.trunc(Number(wait.seconds)) || 0 }
  }
  if (
    node.data.type === SiteAutomationStepType.APPLY_TAG ||
    node.data.type === SiteAutomationStepType.REMOVE_TAG
  ) {
    const tag: ReadProperties<'tag'> = props
    return {
      type:
        node.data.type === SiteAutomationStepType.APPLY_TAG
          ? SiteAutomationStepType.APPLY_TAG
          : SiteAutomationStepType.REMOVE_TAG,
      tag: asText(tag.tag),
    }
  }
  if (node.data.type === SiteAutomationStepType.TELEGRAM) {
    return telegramStep(props)
  }
  if (node.data.type === SiteAutomationStepType.SMS) {
    return smsStep(props)
  }
  if (node.data.type === SiteAutomationStepType.EMAIL) {
    // A message written in the editor is saved as its document alone (the server builds the
    // body; sending both is a 422), and one written as HTML as its body alone. A step that names
    // no Integration ('') leaves it out: it sends through the email default.
    const email: ReadProperties<'email'> = props
    const subject = asText(email.subject)
    const integrationId = asText(email.integrationId)
    const via = integrationId === '' ? {} : { integrationId }
    const doc = docCodec.safeParse(asText(email.doc))
    return doc.success
      ? { type: 'email', ...via, subject, doc: doc.data }
      : { type: 'email', ...via, subject, body: asText(email.body) }
  }
  return null
}

// graphToSteps linearizes the canvas graph: it finds the one Trigger node, reads the trigger,
// exit and goal events off it and walks single outgoing edges from there into ordered steps.
// The Trigger is not a Step. Branches are not supported yet: nodes off the main chain are
// reported via `dropped` so the caller can warn instead of silently losing them. `triggerEvent`
// is null when the canvas does not hold exactly one Trigger.
export function graphToSteps(graph: AutomationGraph): {
  triggerEvent: string | null
  exitEvents: string[]
  goalEvent: string | null
  steps: AutomationStep[]
  dropped: number
} {
  const ordered = linearGraphNodes(graph)
  const [root] = ordered
  if (!root) {
    return { triggerEvent: null, exitEvents: [], goalEvent: null, steps: [], dropped: 0 }
  }
  const steps = ordered.flatMap((node) => {
    const step = nodeToStep(node)
    return step ? [step] : []
  })
  const goalEvent = asText(root.data.properties.goalEvent).trim()
  return {
    triggerEvent: asText(root.data.properties.triggerEvent),
    exitEvents: exitEventsOf(asText(root.data.properties.exitEvents)),
    goalEvent: goalEvent === '' ? null : goalEvent,
    steps,
    dropped: graph.nodes.length - ordered.length,
  }
}

// The canvas order is its edges, not the order in which nodes were created. Save uses this same
// walk to attach server-assigned HTTP step identities to the submitted canvas nodes.
export function linearGraphNodes(graph: AutomationGraph): WorkflowBuilderNode[] {
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
