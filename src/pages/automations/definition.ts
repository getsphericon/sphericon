import type { WBIcon, WorkflowBuilderEdge, WorkflowBuilderNode } from '@workflowbuilder/sdk'

import type { SiteAutomationStep } from '../../generated/site/types.gen.ts'
import {
  APPLY_TAG_NODE_TYPE,
  EMAIL_NODE_ICON,
  EMAIL_NODE_TYPE,
  REMOVE_TAG_NODE_TYPE,
  TAG_NODE_ICON,
  WAIT_NODE_ICON,
  WAIT_NODE_TYPE,
} from './nodes.tsx'

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
}

function stepProperties(step: AutomationStep): Record<string, unknown> {
  if (step.type === 'wait') return { seconds: step.seconds ?? 0 }
  if (step.type === 'apply_tag' || step.type === 'remove_tag') return { tag: step.tag ?? '' }
  return { subject: step.subject ?? '', body: step.body ?? '' }
}

// stepsToGraph builds a linear top-to-bottom chain of nodes for the canvas from
// the stored steps. Positions are synthesized from order, never persisted.
export function stepsToGraph(steps: AutomationStep[]): AutomationGraph {
  const nodes: WorkflowBuilderNode[] = steps.map((step, i) => ({
    id: `step-${i}`,
    type: NODE_RENDERER_TYPE,
    position: { x: 0, y: i * Y_GAP },
    data: {
      type: step.type,
      icon: STEP_ICON[step.type],
      segments: [],
      properties: stepProperties(step),
    },
  }))

  const edges: WorkflowBuilderEdge[] = []
  for (let i = 1; i < steps.length; i++) {
    edges.push({
      id: `edge-${i - 1}-${i}`,
      source: `step-${i - 1}`,
      target: `step-${i}`,
      sourceHandle: SOURCE_HANDLE,
      targetHandle: TARGET_HANDLE,
      type: EDGE_TYPE,
    })
  }

  return { nodes, edges }
}

function asText(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function nodeToStep(node: WorkflowBuilderNode): AutomationStep | null {
  const props = (node.data.properties ?? {}) as Record<string, unknown>
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
    return {
      type: 'email',
      subject: asText(props.subject),
      body: asText(props.body),
    }
  }
  return null
}

// graphToSteps linearizes the canvas graph into ordered steps by walking from
// the entry node (in-degree 0) along single outgoing edges. Branches are not
// supported yet: extra nodes off the main chain are reported via `dropped` so
// the caller can warn instead of silently losing them.
export function graphToSteps(graph: AutomationGraph): { steps: AutomationStep[]; dropped: number } {
  const { nodes, edges } = graph
  if (nodes.length === 0) return { steps: [], dropped: 0 }

  const byId = new Map(nodes.map((n) => [n.id, n]))
  const outgoing = new Map<string, string[]>()
  const indegree = new Map<string, number>(nodes.map((n) => [n.id, 0]))
  for (const e of edges) {
    if (!byId.has(e.source) || !byId.has(e.target)) continue
    outgoing.set(e.source, [...(outgoing.get(e.source) ?? []), e.target])
    indegree.set(e.target, (indegree.get(e.target) ?? 0) + 1)
  }

  const root = nodes.find((n) => (indegree.get(n.id) ?? 0) === 0) ?? nodes[0]
  if (!root) return { steps: [], dropped: 0 }
  const steps: AutomationStep[] = []
  const visited = new Set<string>()
  let current: string | undefined = root.id
  while (current && !visited.has(current)) {
    visited.add(current)
    const node = byId.get(current)
    const step = node ? nodeToStep(node) : null
    if (step) steps.push(step)
    current = outgoing.get(current)?.[0]
  }

  return { steps, dropped: nodes.length - visited.size }
}
