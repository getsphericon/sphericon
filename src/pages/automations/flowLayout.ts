import { graphlib, layout } from '@dagrejs/dagre'
import type { Edge, Node } from '@xyflow/react'

import type { Flow, FlowStep } from './definition.ts'

// The canvas is drawn from the flow on every change, laid out top to bottom by dagre: the Trigger,
// each Step of the root path, and an insertion point on the edge below each of them, where a new
// Step goes. Nothing here is stored or moved by hand: the nodes are neither dragged nor connected.

export const TRIGGER_NODE_ID = 'trigger'

// Every node has a fixed size, so the layout needs no measuring.
const NODE_WIDTH = 280
const NODE_HEIGHT = 64
const INSERT_SIZE = 28
// The room between two nodes along an edge.
const RANK_GAP = 24

type TriggerNode = Node<Record<string, never>, 'trigger'>
export type StepNode = Node<{ step: FlowStep }, 'step'>
// An insertion point puts a new Step at `at` of the root path: 0 is right after the Trigger.
export type InsertNode = Node<{ at: number }, 'insert'>
export type FlowNode = TriggerNode | StepNode | InsertNode

const unplaced = { position: { x: 0, y: 0 } }

function insertNode(at: number): InsertNode {
  return {
    ...unplaced,
    id: `insert-${at}`,
    type: 'insert',
    data: { at },
    width: INSERT_SIZE,
    height: INSERT_SIZE,
  }
}

// layoutFlow draws the flow: its nodes placed top to bottom, and an edge between each two.
export function layoutFlow(flow: Flow): { nodes: FlowNode[]; edges: Edge[] } {
  const chain: FlowNode[] = [
    {
      ...unplaced,
      id: TRIGGER_NODE_ID,
      type: 'trigger',
      data: {},
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    },
    insertNode(0),
    ...flow.path.steps.flatMap((step, i): FlowNode[] => [
      {
        ...unplaced,
        id: step.id,
        type: 'step',
        data: { step },
        width: NODE_WIDTH,
        height: NODE_HEIGHT,
      },
      insertNode(i + 1),
    ]),
  ]
  const edges = chain.slice(1).map((node, i): Edge => ({
    id: `edge-${node.id}`,
    type: 'straight',
    source: chain[i]?.id ?? TRIGGER_NODE_ID,
    target: node.id,
  }))

  const graph = new graphlib.Graph()
  graph.setGraph({ rankdir: 'TB', ranksep: RANK_GAP })
  graph.setDefaultEdgeLabel(() => ({}))
  for (const node of chain) {
    graph.setNode(node.id, { width: node.width ?? 0, height: node.height ?? 0 })
  }
  for (const edge of edges) {
    graph.setEdge(edge.source, edge.target)
  }
  layout(graph)

  // dagre places a node by its center, the canvas by its top-left corner.
  const nodes = chain.map((node): FlowNode => {
    const { x = 0, y = 0 } = graph.node(node.id)
    const position = { x: x - (node.width ?? 0) / 2, y: y - (node.height ?? 0) / 2 }
    return { ...node, position }
  })
  return { nodes, edges }
}
