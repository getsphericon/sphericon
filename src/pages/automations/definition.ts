import { NodeType } from '@workflowbuilder/sdk'
import type { WBIcon, WorkflowBuilderEdge, WorkflowBuilderNode } from '@workflowbuilder/sdk'
import * as z from 'zod'

import type {
  SiteAutomationHttpHeader,
  SiteAutomationStep,
  SiteAutomationVersion,
} from '../../generated/site/types.gen.ts'
import { docCodec } from '../../utils/messageDoc.ts'
import {
  APPLY_TAG_NODE_TYPE,
  EMAIL_NODE_ICON,
  EMAIL_NODE_TYPE,
  HTTP_NODE_ICON,
  HTTP_NODE_TYPE,
  REMOVE_TAG_NODE_TYPE,
  TAG_NODE_ICON,
  TRIGGER_NODE_ICON,
  TRIGGER_NODE_TYPE,
  WAIT_NODE_ICON,
  WAIT_NODE_TYPE,
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
  wait: WAIT_NODE_ICON,
  apply_tag: TAG_NODE_ICON,
  remove_tag: TAG_NODE_ICON,
  http: HTTP_NODE_ICON,
}

// The server never returns an HTTP step's URL, header values or signing secret, so the
// canvas holds them blank: a blank field is "keep what is stored". Header names are
// shown as "Name:" lines for the same reason.
function stepProperties(step: AutomationStep): Record<string, unknown> {
  if (step.type === 'http') {
    return {
      stepId: step.id ?? '',
      url: '',
      headers: (step.headers ?? []).map((h) => `${h.name}:`).join('\n'),
      body: step.body ?? '',
      secret: '',
    }
  }
  if (step.type === 'wait') {
    return { seconds: step.seconds ?? 0 }
  }
  if (step.type === 'apply_tag' || step.type === 'remove_tag') {
    return { tag: step.tag ?? '' }
  }
  return {
    subject: step.subject ?? '',
    body: step.body ?? '',
    doc: step.doc ? z.encode(docCodec, step.doc) : '',
  }
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

// parseHeaders reads "Name: value" lines; a line with no value after the colon keeps the
// stored value of the header with that name.
function parseHeaders(text: string): SiteAutomationHttpHeader[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')
    .map((line) => {
      const at = line.indexOf(':')
      const name = (at === -1 ? line : line.slice(0, at)).trim()
      const value = at === -1 ? '' : line.slice(at + 1).trim()
      return value === '' ? { name } : { name, value }
    })
}

function nodeToStep(node: WorkflowBuilderNode): AutomationStep | null {
  const props: Record<string, unknown> = node.data.properties
  if (node.data.type === HTTP_NODE_TYPE) {
    const id = asText(props.stepId)
    const url = asText(props.url).trim()
    const secret = asText(props.secret)
    return {
      type: 'http',
      body: asText(props.body),
      headers: parseHeaders(asText(props.headers)),
      ...(id === '' ? {} : { id }),
      ...(url === '' ? {} : { url }),
      ...(secret === '' ? {} : { secret }),
    }
  }
  if (node.data.type === WAIT_NODE_TYPE) {
    return { type: 'wait', seconds: Math.trunc(Number(props.seconds)) || 0 }
  }
  if (node.data.type === APPLY_TAG_NODE_TYPE || node.data.type === REMOVE_TAG_NODE_TYPE) {
    return {
      type: node.data.type === APPLY_TAG_NODE_TYPE ? 'apply_tag' : 'remove_tag',
      tag: asText(props.tag),
    }
  }
  if (node.data.type === EMAIL_NODE_TYPE) {
    // A message written in the editor is saved as its document alone (the server builds the
    // body; sending both is a 422), and one written as HTML as its body alone.
    const subject = asText(props.subject)
    const doc = docCodec.safeParse(asText(props.doc))
    return doc.success
      ? { type: 'email', subject, doc: doc.data }
      : { type: 'email', subject, body: asText(props.body) }
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
  const { nodes, edges } = graph
  const triggers = nodes.filter((n) => n.data.type === TRIGGER_NODE_TYPE)
  const [root] = triggers
  if (triggers.length !== 1 || !root) {
    return { triggerEvent: null, exitEvents: [], goalEvent: null, steps: [], dropped: 0 }
  }

  const byId = new Map(nodes.map((n) => [n.id, n]))
  const next = new Map<string, string>()
  for (const e of edges) {
    if (byId.has(e.source) && byId.has(e.target) && !next.has(e.source)) {
      next.set(e.source, e.target)
    }
  }

  const steps: AutomationStep[] = []
  const visited = new Set<string>()
  let current: string | undefined = root.id
  while (current && !visited.has(current)) {
    visited.add(current)
    const node = byId.get(current)
    const step = node ? nodeToStep(node) : null
    if (step) {
      steps.push(step)
    }
    current = next.get(current)
  }

  const goalEvent = asText(root.data.properties.goalEvent).trim()
  return {
    triggerEvent: asText(root.data.properties.triggerEvent),
    exitEvents: exitEventsOf(asText(root.data.properties.exitEvents)),
    goalEvent: goalEvent === '' ? null : goalEvent,
    steps,
    dropped: nodes.length - visited.size,
  }
}
